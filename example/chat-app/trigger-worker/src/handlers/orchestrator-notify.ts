import { Request, Response } from 'express';
import { AzureOpenAI } from 'openai';
import { TriggerHandler, TriggerClient, WebhookPayload } from '@syntrix/client';
import { AgentTask, SubAgentMessage, Message, ToolCall } from '../types';
import { generateShortId } from '../utils';

const openai = new AzureOpenAI({
  endpoint: process.env.AZURE_OPENAI_ENDPOINT,
  apiKey: process.env.AZURE_OPENAI_API_KEY,
  apiVersion: '2024-05-01-preview',
});

/**
 * Fetch chat history (messages + tool calls) in chronological order.
 */
async function fetchHistory(syntrix: TriggerClient, chatPath: string): Promise<Message[]> {
  // 1. Fetch Messages
  const messages = await syntrix
    .collection<Message>(`${chatPath}/messages`)
    .orderBy('createdAt', 'asc')
    .get();

  // 2. Fetch Tool Calls
  const toolCalls = await syntrix
    .collection<ToolCall>(`${chatPath}/tool-calls`)
    .orderBy('createdAt', 'asc')
    .get();

  // 3. Merge and Format
  const history: Message[] = [];

  const allEvents = [
    ...messages.map((m: Message) => ({ type: 'message' as const, data: m })),
    ...toolCalls.map((t: ToolCall) => ({ type: 'tool' as const, data: t }))
  ].sort((a, b) => a.data.createdAt - b.data.createdAt);

  for (const event of allEvents) {
    if (event.type === 'message') {
      const m = event.data as Message;
      history.push({
        id: m.id,
        role: m.role,
        content: m.content,
        createdAt: m.createdAt
      });
    } else {
      const t = event.data as ToolCall;
      history.push({
        id: t.id,
        role: 'assistant',
        content: null,
        tool_calls: [{
          id: t.id,
          type: 'function',
          function: {
            name: t.toolName,
            arguments: JSON.stringify(t.args)
          }
        }],
        createdAt: t.createdAt
      });

      if (t.status === 'success' && t.result) {
        history.push({
          id: `${t.id}-result`,
          role: 'tool',
          tool_call_id: t.id,
          content: t.result,
          createdAt: t.createdAt + 1
        });
      }
    }
  }

  return history;
}

export const orchestratorNotifyHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received Orchestrator Notify trigger: ${payload.triggerId}`);

    const task = payload.after as AgentTask;
    if (!task || task.status !== 'waiting' || !task.subAgentId) {
      res.status(200).send('Ignored');
      return;
    }

    // collection: users/demo-user/orch-chats/chat-1/tasks
    const parts = payload.collection.split('/');
    const userId = parts[1];
    const chatId = parts[3];

    if (!payload.preIssuedToken) {
      console.error('Missing preIssuedToken');
      res.status(401).send('Unauthorized');
      return;
    }

    const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL || 'http://localhost:8080');
    const syntrix = handler.syntrix;

    console.log(`[OrchestratorNotify] ChatID: ${chatId}, TaskID: ${task.id}`);
    const chatPath = `users/${userId}/orch-chats/${chatId}`;

    // Fetch latest message from sub-agent
    const messages = await syntrix
      .collection<SubAgentMessage>(`users/${userId}/orch-chats/${chatId}/sub-agents/${task.subAgentId}/messages`)
      .orderBy('createdAt', 'desc')
      .limit(1)
      .get();

    let question = "The agent needs more information.";
    if (messages.length > 0 && messages[0].role === 'assistant') {
      question = messages[0].content;
    }

    // --- Proactive Orchestration ---
    // Check if we can answer this question automatically using chat history.

    const history = await fetchHistory(syntrix, chatPath);
    const systemPrompt = `You are the Orchestrator.
    A sub-agent is asking a question: "${question}".
    Check the conversation history below.
    If the answer is present and unambiguous, use the 'reply_to_agent' tool to answer it.
    If the answer is NOT present, use the 'ask_user' tool to notify the user.
    `;

    const completion = await openai.chat.completions.create({
      model: process.env.AZURE_OPENAI_MODEL || 'gpt-4o',
      messages: [
        { role: 'system', content: systemPrompt },
        ...history.map(m => ({
          role: m.role as any,
          content: m.content
        }))
      ],
      tools: [
        {
          type: 'function',
          function: {
            name: 'reply_to_agent',
            description: 'Answer the agent directly.',
            parameters: {
              type: 'object',
              properties: {
                answer: { type: 'string' }
              },
              required: ['answer']
            }
          }
        },
        {
          type: 'function',
          function: {
            name: 'ask_user',
            description: 'Ask the user for input.',
            parameters: {
              type: 'object',
              properties: {},
            }
          }
        }
      ]
    });

    const message = completion.choices[0].message;
    if (message.tool_calls && message.tool_calls.length > 0) {
      const toolCall = message.tool_calls[0];
      const args = JSON.parse(toolCall.function.arguments);

      if (toolCall.function.name === 'reply_to_agent') {
        const answer = args.answer;
        console.log(`[OrchestratorNotify] Auto-answering agent: ${answer}`);

        // 1. Update SubAgent status
        await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${task.subAgentId}`).update({
          status: 'active',
          updatedAt: Date.now()
        });

        // 2. Inject User Message (Auto-Answer)
        const msgId = generateShortId();
        await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${task.subAgentId}/messages/${msgId}`).set({
          id: msgId,
          userId,
          subAgentId: task.subAgentId,
          role: 'user',
          content: answer,
          createdAt: Date.now()
        });

        // 3. Update Task
        await syntrix.doc(`users/${userId}/orch-chats/${chatId}/tasks/${task.id}`).update({
          status: 'running',
          updatedAt: Date.now()
        });

        // 4. Log to Main Chat (Optional)
        const notifyId = generateShortId();
        await syntrix.doc(`${chatPath}/messages/${notifyId}`).set({
          id: notifyId,
          role: 'assistant',
          content: `(Auto-replied to Agent: "${answer}")`,
          createdAt: Date.now()
        });

      } else {
        // ask_user
        const content = `**Agent ${task.name} Needs Input:**\n\n${question}`;
        const msgId = generateShortId();
        await syntrix.doc(`${chatPath}/messages/${msgId}`).set({
          id: msgId,
          role: 'assistant',
          content,
          createdAt: Date.now()
        });
      }
    } else {
      // Fallback
      const content = `**Agent ${task.name} Needs Input:**\n\n${question}`;
      const msgId = generateShortId();
      await syntrix.doc(`${chatPath}/messages/${msgId}`).set({
        id: msgId,
        role: 'assistant',
        content,
        createdAt: Date.now()
      });
    }

    res.status(200).send('OK');
  } catch (error) {
    console.error('Orchestrator Notify Handler Error:', error);
    res.status(500).send('Internal Server Error');
  }
};

import { Request, Response } from 'express';
import { AzureOpenAI } from 'openai';
import { TriggerHandler, WebhookPayload } from '@syntrix/client';
import { AgentTask, SubAgentMessage, Message, ToolCall } from '../types';
import { generateShortId } from '../utils';

const openai = new AzureOpenAI({
  endpoint: process.env.AZURE_OPENAI_ENDPOINT,
  apiKey: process.env.AZURE_OPENAI_API_KEY,
  apiVersion: '2024-05-01-preview',
});

const ORCHESTRATOR_TOOLS = [
  {
    type: 'function' as const,
    function: {
      name: 'create_agent',
      description: 'Create a new sub-agent to handle a complex task.',
      parameters: {
        type: 'object',
        properties: {
          type: {
            type: 'string',
            enum: ['researcher', 'general'],
            description: 'The type of agent to create.',
          },
          instruction: {
            type: 'string',
            description: 'Detailed instruction for the agent.',
          },
        },
        required: ['type', 'instruction'],
      },
    },
  },
  {
    type: 'function' as const,
    function: {
      name: 'reactivate_agent',
      description: 'Reactivate an existing sub-agent with new instructions.',
      parameters: {
        type: 'object',
        properties: {
          subAgentId: {
            type: 'string',
            description: 'The ID of the sub-agent to reactivate.',
          },
          instruction: {
            type: 'string',
            description: 'The new instruction for the agent.',
          },
        },
        required: ['subAgentId', 'instruction'],
      },
    },
  },
];

export const orchestratorHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received Orchestrator trigger: ${payload.triggerId}`);

    // Parse Context
    // collection: users/demo-user/orch-chats/chat-1/messages
    const parts = payload.collection.split('/');
    const userId = parts[1];
    const chatId = parts[3];
    if (!payload.preIssuedToken) {
      console.error('Missing preIssuedToken');
      res.status(401).send('Unauthorized');
      return;
    }

    const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL || 'http://localhost:8080/api/v1');
    const syntrix = handler.syntrix;

    console.log(`[Orchestrator] ChatID: ${chatId}`);
    const chatPath = `users/${userId}/orch-chats/${chatId}`;

    // Fetch History
    const baseMessages = await syntrix
      .collection<Message>(`${chatPath}/messages`)
      .orderBy('createdAt', 'asc')
      .get();

    const toolCalls = await syntrix
      .collection<ToolCall>(`${chatPath}/tool-calls`)
      .orderBy('createdAt', 'asc')
      .get();

    const history: Message[] = [];
    const allEvents = [
      ...baseMessages.map((m: Message) => ({ type: 'message' as const, data: m })),
      ...toolCalls.map((t: ToolCall) => ({ type: 'tool' as const, data: t }))
    ].sort((a, b) => a.data.createdAt - b.data.createdAt);

    for (const event of allEvents) {
      if (event.type === 'message') {
        const m = event.data as Message;
        history.push({
          id: m.id,
          role: m.role,
          content: m.content,
          tool_calls: m.tool_calls,
          tool_call_id: m.tool_call_id,
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

    // Check if Trigger Message is Last
    if (history.length > 0) {
      const lastMsg = history[history.length - 1];
      const triggerMsgId = (payload.after as any).id;
      if (lastMsg.id !== triggerMsgId) {
        console.log(`[Orchestrator] Trigger message ${triggerMsgId} is not the last message (Last: ${lastMsg.id}). Skipping.`);
        res.status(200).send('Skipped (Not Last Message)');
        return;
      }
    }

    // Fetch Recent Tasks (Agents) to provide context to Orchestrator
    // We query 'tasks' instead of 'sub-agents' because tasks contain the semantic info (name, instruction).
    const recentTasks = await syntrix
      .collection<AgentTask>(`users/${userId}/orch-chats/${chatId}/tasks`)
      .orderBy('updatedAt', 'desc')
      .limit(5)
      .get();

    // Fetch Waiting Tasks (Agents needing input)
    const waitingTasks = recentTasks.filter((t: AgentTask) => t.status === 'waiting');

    // For each waiting task, get the last question
    const waitingAgentsInfo = await Promise.all(waitingTasks.map(async (task: AgentTask) => {
      if (!task.subAgentId) return null;
      const msgs = await syntrix
        .collection<SubAgentMessage>(`users/${userId}/orch-chats/${chatId}/sub-agents/${task.subAgentId}/messages`)
        .orderBy('createdAt', 'desc')
        .limit(1)
        .get();
      const question = (msgs.length > 0 && msgs[0].role === 'assistant') ? msgs[0].content : 'Unknown question';
      return {
        subAgentId: task.subAgentId,
        name: task.name,
        question
      };
    }));
    const validWaitingAgents = waitingAgentsInfo.filter((a: typeof waitingAgentsInfo[number]) => a !== null);

    // --- Context Injection for Caching Optimization ---
    // 1. Static System Prompt (Cacheable Prefix)
    const staticSystemPrompt = `You are the Orchestrator. Your job is to manage the conversation.
    You can answer directly if the user's request is simple.
    If the request is complex (e.g. "research X", "write a report"), delegate it to a sub-agent.

    Routing Rules:
    1. If the user's message answers a waiting agent's question, use 'reply_to_agent'.
    2. If the user is following up on a previous agent's work (not waiting), use 'reactivate_agent'.
    3. If it's a new complex task, use 'create_agent'.
    4. Otherwise, reply directly.`;

    // 2. Dynamic Context (Injected at the end)
    const dynamicContext = `Current System State:

    Existing Sub-Agents (Tasks):
    ${recentTasks.filter((t: AgentTask) => t.subAgentId).map((t: AgentTask) => `- Name: ${t.name}, AgentID: ${t.subAgentId}, Status: ${t.status}, Instruction: ${t.instruction}`).join('\n')}

    WAITING AGENTS (These agents are waiting for user input):
    ${validWaitingAgents.map(a => `- Agent: ${a!.name} (ID: ${a!.subAgentId}) is asking: "${a!.question}"`).join('\n')}

    (Use this state to decide on the routing for the latest user message)`;

    // 3. Construct Messages (Split History)
    // [Static System] + [History (0...N-1)] + [Dynamic Context] + [Latest User Message (N)]
    const previousHistory = history.slice(0, -1);
    const lastMsg = history[history.length - 1];

    const messages = [
        { role: 'system', content: staticSystemPrompt },
        ...previousHistory.map(m => ({
          role: m.role as any,
          content: m.content,
          tool_calls: m.tool_calls,
          tool_call_id: m.tool_call_id
        })),
        { role: 'system', content: dynamicContext },
        {
          role: lastMsg.role as any,
          content: lastMsg.content,
          tool_calls: lastMsg.tool_calls,
          tool_call_id: lastMsg.tool_call_id
        }
    ];

    const completion = await openai.chat.completions.create({
      model: process.env.AZURE_OPENAI_MODEL || 'gpt-4o',
      messages: messages as any,
      tools: [
          ...ORCHESTRATOR_TOOLS,
          {
            type: 'function' as const,
            function: {
              name: 'reply_to_agent',
              description: 'Provide input to a waiting sub-agent.',
              parameters: {
                type: 'object',
                properties: {
                  subAgentId: { type: 'string' },
                  content: { type: 'string', description: 'The answer/input for the agent.' }
                },
                required: ['subAgentId', 'content']
              }
            }
          }
      ],
    });

    const message = completion.choices[0].message;

    if (message.tool_calls && message.tool_calls.length > 0) {
        const toolCall = message.tool_calls[0];
        const args = JSON.parse(toolCall.function.arguments);

        if (toolCall.function.name === 'create_agent') {
            // Create Task
            const taskId = generateShortId();
            const task: AgentTask = {
                id: taskId,
                userId,
                chatId,
                type: 'agent',
                name: `agent:${args.type}`,
                instruction: args.instruction,
                triggerMessageId: payload.documentId || (payload.after as any)?.id || '', // The user message ID
                status: 'pending',
                createdAt: Date.now(),
                updatedAt: Date.now()
            };
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/tasks/${taskId}`).set(task);
        } else if (toolCall.function.name === 'reactivate_agent') {
            // Reactivate Agent
            const subAgentId = args.subAgentId;
            // 1. Update SubAgent status
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).update({
              status: 'active',
              updatedAt: Date.now()
            });

            // 2. Inject User Message into SubAgent
            const msgId = generateShortId();
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${msgId}`).set({
              id: msgId,
              userId,
              subAgentId,
              role: 'user',
              content: args.instruction,
              createdAt: Date.now()
            });

            // 3. Update parent task status
            const tasks = await syntrix
              .collection<AgentTask>(`users/${userId}/orch-chats/${chatId}/tasks`)
              .where('subAgentId', '==', subAgentId)
              .get();
            if (tasks.length > 0) {
              await syntrix.doc(`users/${userId}/orch-chats/${chatId}/tasks/${tasks[0].id}`).update({
                status: 'running',
                updatedAt: Date.now()
              });
            }
        } else if (toolCall.function.name === 'reply_to_agent') {
            // Reply to Agent
            const subAgentId = args.subAgentId;
            const content = args.content;

            // 1. Update SubAgent status
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).update({
              status: 'active',
              updatedAt: Date.now()
            });

            // 2. Inject User Message (from Orchestrator) into SubAgent
            const msgId = generateShortId();
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${msgId}`).set({
              id: msgId,
              userId,
              subAgentId,
              role: 'user',
              content,
              createdAt: Date.now()
            });

            // 3. Update parent task status
            const tasks = await syntrix
              .collection<AgentTask>(`users/${userId}/orch-chats/${chatId}/tasks`)
              .where('subAgentId', '==', subAgentId)
              .get();
            if (tasks.length > 0) {
              await syntrix.doc(`users/${userId}/orch-chats/${chatId}/tasks/${tasks[0].id}`).update({
                status: 'running',
                updatedAt: Date.now()
              });
            }

            // 4. Notify User (Optional but good for UX)
            const notifyId = generateShortId();
            await syntrix.doc(`${chatPath}/messages/${notifyId}`).set({
              id: notifyId,
              role: 'assistant',
              content: `(Forwarded to Agent: "${content}")`,
              createdAt: Date.now()
            });
        }
    } else if (message.content) {
        // Direct Reply
        const msgId = generateShortId();
        await syntrix.doc(`${chatPath}/messages/${msgId}`).set({
          id: msgId,
          role: 'assistant',
          content: message.content,
          createdAt: Date.now()
        });
    }

    res.status(200).send('OK');
  } catch (error) {
    console.error('Orchestrator Handler Error:', error);
    res.status(500).send('Internal Server Error');
  }
};

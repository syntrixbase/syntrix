import { Request, Response } from 'express';
import { AzureOpenAI } from 'openai';
import { TriggerHandler, WebhookPayload } from '@syntrix/client';
import { SubAgentMessage, SubAgentToolCall, AgentTask, SubAgent } from '../types';
import { generateShortId } from '../utils';

const openai = new AzureOpenAI({
  endpoint: process.env.AZURE_OPENAI_ENDPOINT,
  apiKey: process.env.AZURE_OPENAI_API_KEY,
  apiVersion: '2024-05-01-preview',
});

const AGENT_TOOLS = [
  {
    type: 'function' as const,
    function: {
      name: 'tavily_search',
      description: 'Search the web.',
      parameters: {
        type: 'object',
        properties: {
          query: { type: 'string' },
        },
        required: ['query'],
      },
    },
  },
  {
    type: 'function' as const,
    function: {
      name: 'final_answer',
      description: 'Provide the final answer to the user.',
      parameters: {
        type: 'object',
        properties: {
          answer: { type: 'string' },
        },
        required: ['answer'],
      },
    },
  },
  {
    type: 'function' as const,
    function: {
      name: 'ask_user',
      description: 'Ask the user for clarification.',
      parameters: {
        type: 'object',
        properties: {
          question: { type: 'string' },
        },
        required: ['question'],
      },
    },
  },
];

export const agentLoopHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received Agent Loop trigger: ${payload.triggerId}`);

    const message = payload.after as SubAgentMessage;
    // collection: users/demo-user/orch-chats/chat-1/sub-agents/agent-1/messages
    const parts = payload.collection.split('/');
    const userId = parts[1];
    const chatId = parts[3];
    const subAgentId = parts[5];
    if (!payload.preIssuedToken) {
      console.error('Missing preIssuedToken');
      res.status(401).send('Unauthorized');
      return;
    }

    const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL || 'http://localhost:8080/api/v1');
    const syntrix = handler.syntrix;

    console.log(`[AgentLoop] ChatID: ${chatId}, AgentID: ${subAgentId}`);

    // 1. Pending Check
    const pendingTools = await syntrix
      .collection<SubAgentToolCall>(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/tool-calls`)
      .where('status', '==', 'pending')
      .get();

    if (pendingTools.length > 0) {
        console.log('Pending tools found, skipping LLM call.');
        res.status(200).send('Skipped (Pending Tools)');
        return;
    }

    // 2. Fetch History
    const messages = await syntrix
      .collection<SubAgentMessage>(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages`)
      .orderBy('createdAt', 'asc')
      .get();

    // 2.1 Check if Trigger Message is Last
    // This prevents race conditions where multiple tool outputs trigger multiple runs.
    // Only the run triggered by the LATEST message should proceed.
    if (messages.length > 0) {
        const lastMsg = messages[messages.length - 1];
        if (lastMsg.id !== message.id) {
             console.log(`[AgentLoop] Trigger message ${message.id} is not the last message (Last: ${lastMsg.id}). Skipping.`);
             res.status(200).send('Skipped (Not Last Message)');
             return;
        }
    }

    // 2.5 Check for Missing Tool Outputs
    // Find the last assistant message with tool_calls
    let lastAssistantWithToolsIndex = -1;
    for (let i = messages.length - 1; i >= 0; i--) {
        const msg = messages[i];
        if (msg.role === 'assistant' && msg.toolCalls && msg.toolCalls.length > 0) {
            lastAssistantWithToolsIndex = i;
            break;
        }
    }

    if (lastAssistantWithToolsIndex !== -1) {
        const assistantMsg = messages[lastAssistantWithToolsIndex];
        // We know toolCalls exists because of the check above, but TS needs help
        const toolCalls = assistantMsg.toolCalls || [];
        const toolCallIds = toolCalls.map((tc: any) => tc.id);

        // Check if we have tool messages for all these IDs
        const toolResponses = messages.slice(lastAssistantWithToolsIndex + 1).filter((m: SubAgentMessage) => m.role === 'tool');
        const respondedIds = new Set(toolResponses.map((m: SubAgentMessage) => m.toolCallId));

        const missingIds = toolCallIds.filter((id: string) => !respondedIds.has(id));

        if (missingIds.length > 0) {
            console.log(`Missing tool outputs for IDs: ${missingIds.join(', ')}. Skipping LLM call.`);
            res.status(200).send('Skipped (Missing Tool Outputs)');
            return;
        }
    }

    // 3. Idempotency Lock (DB-based)
    // We use the last message ID to create a unique run record.
    // If multiple triggers fire for the same state, only one will succeed in creating this record.
    const runId = `run_${message.id}`;
    try {
        await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/runs/${runId}`).set({
          id: runId,
          triggerMsgId: message.id,
          createdAt: Date.now(),
          status: 'processing'
        });
      } catch (error: any) {
        if (error?.response?.status === 409) {
          console.log(`[AgentLoop] Run lock ${runId} already exists. Skipping duplicate execution.`);
          res.status(200).send('Skipped (Duplicate Run)');
          return;
        }
        throw error;
      }

    // 4. Call LLM
    const openAIMessages = messages.map((m: SubAgentMessage) => {
        const msg: any = {
            role: m.role,
            content: m.content
        };
        if (m.toolCalls && m.toolCalls.length > 0) {
            msg.tool_calls = m.toolCalls;
        }
        if (m.toolCallId) {
            msg.tool_call_id = m.toolCallId;
        }
        return msg;
    });

    const completion = await openai.chat.completions.create({
      model: process.env.AZURE_OPENAI_MODEL || 'gpt-4o',
      messages: openAIMessages,
      tools: AGENT_TOOLS,
    });

    const responseMsg = completion.choices[0].message;

    // 4. Handle Response
    if (responseMsg.tool_calls && responseMsg.tool_calls.length > 0) {
        // Check for final_answer or ask_user
        const finalAnswerCall = responseMsg.tool_calls.find(tc => tc.function.name === 'final_answer');
        const askUserCall = responseMsg.tool_calls.find(tc => tc.function.name === 'ask_user');

        if (finalAnswerCall) {
            // Final Answer
            const args = JSON.parse(finalAnswerCall.function.arguments);
            const answer = args.answer;

            // Create Assistant Message
            const msgId = generateShortId();
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${msgId}`).set({
              id: msgId,
              userId,
              subAgentId,
              role: 'assistant',
              content: answer,
              createdAt: Date.now()
            });

            // Update SubAgent -> completed
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).update({
              status: 'completed',
              updatedAt: Date.now()
            });

            // Update Task -> success
            const subAgent = await syntrix.doc<SubAgent>(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).get();
            if (subAgent) {
              await syntrix.doc(`users/${userId}/orch-chats/${chatId}/tasks/${subAgent.taskId}`).update({
                status: 'success',
                result: answer,
                updatedAt: Date.now()
              });
            }

        } else if (askUserCall) {
            // Ask User
            const args = JSON.parse(askUserCall.function.arguments);
            const question = args.question;

            // Create Assistant Message
            const msgId = generateShortId();
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${msgId}`).set({
              id: msgId,
              userId,
              subAgentId,
              role: 'assistant',
              content: question,
              createdAt: Date.now()
            });

            // Update SubAgent -> waiting_for_user
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).update({
              status: 'waiting_for_user',
              updatedAt: Date.now()
            });

            // Update Task -> waiting
            const subAgent = await syntrix.doc<SubAgent>(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).get();
            if (subAgent) {
              await syntrix.doc(`users/${userId}/orch-chats/${chatId}/tasks/${subAgent.taskId}`).update({
                status: 'waiting',
                updatedAt: Date.now()
              });
            }

        } else {
            // Regular Tool Calls (e.g. tavily_search)

            // Create Assistant Message with tool_calls
            const assistantMsgId = generateShortId();
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${assistantMsgId}`).set({
              id: assistantMsgId,
              userId,
              subAgentId,
              role: 'assistant',
              content: null,
              toolCalls: responseMsg.tool_calls,
              createdAt: Date.now()
            });

            // Create Tool Call Documents for ALL tool calls
            for (const tc of responseMsg.tool_calls) {
                const tcArgs = JSON.parse(tc.function.arguments);
                await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/tool-calls/${tc.id}`).set({
                  id: tc.id,
                  userId,
                  subAgentId,
                  toolName: tc.function.name,
                  args: tcArgs,
                  status: 'pending',
                  createdAt: Date.now()
                });
            }
        }
    } else if (responseMsg.content) {
        // Just a thought or chat (should be avoided by system prompt, but handle it)
            const msgId = generateShortId();
            await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${msgId}`).set({
              id: msgId,
              userId,
              subAgentId,
              role: 'assistant',
              content: responseMsg.content,
              createdAt: Date.now()
            });
    }

    res.status(200).send('OK');
  } catch (error) {
    console.error('Agent Loop Handler Error:', error);
    res.status(500).send('Internal Server Error');
  }
};

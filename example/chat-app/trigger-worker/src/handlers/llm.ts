import { Request, Response } from 'express';
import { AzureOpenAI } from 'openai';
import { TriggerHandler, WebhookPayload } from '@syntrix/client';
import { Message, ToolCall } from '../types';

const openai = new AzureOpenAI({
  endpoint: process.env.AZURE_OPENAI_ENDPOINT,
  apiKey: process.env.AZURE_OPENAI_API_KEY,
  apiVersion: '2024-05-01-preview',
});

const TOOLS = [
  {
    type: 'function' as const,
    function: {
      name: 'tavily_search',
      description: 'Search the web for current information.',
      parameters: {
        type: 'object',
        properties: {
          query: {
            type: 'string',
            description: 'The search query.',
          },
          searchDepth: {
            type: 'string',
            enum: ['basic', 'advanced'],
            description: 'Search depth: "basic" for faster results, "advanced" for more comprehensive results. Default is "basic".',
          },
          maxResults: {
            type: 'number',
            description: 'Maximum number of results to return. Default is 5.',
          },
          includeDomains: {
            type: 'array',
            items: { type: 'string' },
            description: 'List of domains to include in search.',
          },
          excludeDomains: {
            type: 'array',
            items: { type: 'string' },
            description: 'List of domains to exclude from search.',
          },
        },
        required: ['query'],
      },
    },
  },
];

export const llmHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received LLM trigger: ${payload.triggerId}`);

    // 1. Parse Context
    // collection: users/demo-user/orch-chats/chat-1/messages
    // or users/demo-user/orch-chats/chat-1/toolcall
    const parts = payload.collection.split('/');
    // parts: ["users", "demo-user", "chats", "chat-1", "messages"]
    if (parts.length < 5) {
      console.error('Invalid collection path:', payload.collection);
      res.status(400).send('Invalid path');
      return;
    }

    const userId = parts[1];
    const chatId = parts[3];
    const chatPath = `users/${userId}/orch-chats/${chatId}`;
    if (!payload.preIssuedToken) {
      console.error('Missing preIssuedToken');
      res.status(401).send('Unauthorized');
      return;
    }

    const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL || 'http://localhost:8080/api/v1');
    const syntrix = handler.syntrix;

    // 2. Reconstruct History
    const messages = await syntrix
        .collection<Message>(`${chatPath}/messages`)
        .orderBy('createdAt', 'asc')
        .get();

    const toolCalls = await syntrix
        .collection<ToolCall>(`${chatPath}/tool-calls`)
        .orderBy('createdAt', 'asc')
        .get();

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

    // 3. Safety Check (Max Turns)
    // Count how many tool calls are in the history
    const toolCallCount = history.filter(m => m.role === 'assistant' && m.tool_calls).length;
    if (toolCallCount > 5) {
      console.warn('Max tool turns reached. Forcing reply.');
      // Append system message to force stop
      history.push({
        id: 'system-stop',
        role: 'system',
        content: 'You have reached the maximum number of tool calls. Please provide a final answer based on the information you have.',
        createdAt: Date.now()
      });
    }

    // 4. Call OpenAI
    const completion = await openai.chat.completions.create({
      model: process.env.AZURE_OPENAI_MODEL || 'gpt-4o',
      messages: [
        { role: 'system', content: 'You are a helpful assistant. You have access to tools. Use them when needed.' },
        ...history.map(m => ({
          role: m.role as any,
          content: m.content,
          tool_calls: m.tool_calls,
          tool_call_id: m.tool_call_id
        }))
      ],
      tools: toolCallCount > 5 ? undefined : TOOLS, // Disable tools if limit reached
    });

    const choice = completion.choices[0];
    const message = choice.message;

    // 5. Handle Response
    if (message.tool_calls && message.tool_calls.length > 0) {
      // Handle Tool Calls
      for (const toolCall of message.tool_calls) {
        await syntrix
          .doc(`${chatPath}/tool-calls/${toolCall.id}`)
          .set({
            id: toolCall.id,
            toolName: toolCall.function.name,
            args: JSON.parse(toolCall.function.arguments),
            status: 'pending',
            createdAt: Date.now()
          });
      }
    } else if (message.content) {
      // Handle Text Reply
      const id = `msg_${Date.now()}`;
      await syntrix.doc(`${chatPath}/messages/${id}`).set({
        id,
        role: 'assistant',
        content: message.content,
        createdAt: Date.now()
      });
    }

    res.status(200).send('OK');
  } catch (error) {
    console.error('LLM Handler Error:', error);
    res.status(500).send('Internal Server Error');
  }
};

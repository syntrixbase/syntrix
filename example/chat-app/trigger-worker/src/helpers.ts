/**
 * Helper functions that wrap TriggerClient for common operations.
 * These provide the same convenience methods as the old custom SyntrixClient.
 */

import { TriggerClient } from '@syntrix/client';
import { Message, ToolCall } from './types';
import { generateShortId } from './utils';

/**
 * Fetch chat history (messages + tool calls) in chronological order.
 */
export async function fetchHistory(syntrix: TriggerClient, chatPath: string): Promise<Message[]> {
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

  // We need to interleave them based on createdAt
  const allEvents = [
    ...messages.map(m => ({ type: 'message' as const, data: m })),
    ...toolCalls.map(t => ({ type: 'tool' as const, data: t }))
  ].sort((a, b) => a.data.createdAt - b.data.createdAt);

  for (const event of allEvents) {
    if (event.type === 'message') {
      const m = event.data;
      history.push({
        id: m.id,
        role: m.role,
        content: m.content,
        createdAt: m.createdAt
      });
    } else {
      const t = event.data;
      // Tool Call (Request) -> Assistant Message with tool_calls
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

      // Tool Result -> Tool Message
      if (t.status === 'success' && t.result) {
        history.push({
          id: `${t.id}-result`,
          role: 'tool',
          tool_call_id: t.id,
          content: t.result,
          createdAt: t.createdAt + 1 // Ensure it comes after the call
        });
      }
    }
  }

  return history;
}

/**
 * Post an assistant message to the chat.
 */
export async function postMessage(syntrix: TriggerClient, chatPath: string, content: string): Promise<void> {
  const id = generateShortId();
  await syntrix.doc(`${chatPath}/messages/${id}`).set({
    id,
    role: 'assistant',
    content,
    createdAt: Date.now()
  });
}

/**
 * Update a tool call document with success result.
 */
export async function updateToolCall(syntrix: TriggerClient, chatPath: string, callId: string, result: string): Promise<void> {
  await syntrix.doc(`${chatPath}/tool-calls/${callId}`).update({
    status: 'success',
    result
  });
}

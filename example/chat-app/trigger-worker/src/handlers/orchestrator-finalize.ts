import { Request, Response } from 'express';
import { TriggerHandler, WebhookPayload } from '@syntrix/client';
import { AgentTask } from '../types';
import { generateShortId } from '../utils';

export const orchestratorFinalizeHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received Orchestrator Finalize trigger: ${payload.triggerId}`);

    const task = payload.after as AgentTask;
    if (!task || task.status !== 'success' || !task.result) {
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

    console.log(`[OrchestratorFinalize] ChatID: ${chatId}, TaskID: ${task.id}`);
    const chatPath = `users/${userId}/orch-chats/${chatId}`;

    const content = `**Agent ${task.name} Completed:**\n\n${task.result}`;

    // Idempotency Check: Prevent duplicate completion messages
    const history = await syntrix
      .collection<{ id: string; role: string; content: string | null; createdAt: number }>(`${chatPath}/messages`)
      .orderBy('createdAt', 'desc')
      .limit(1)
      .get();

    if (history.length > 0) {
      const lastMsg = history[0];
      if (lastMsg.role === 'assistant' && lastMsg.content && lastMsg.content.startsWith(`**Agent ${task.name} Completed:**`)) {
        console.log(`[OrchestratorFinalize] Duplicate completion message detected. Skipping.`);
        res.status(200).send('Skipped (Duplicate)');
        return;
      }
    }

    // Post completion message
    const msgId = generateShortId();
    await syntrix.doc(`${chatPath}/messages/${msgId}`).set({
      id: msgId,
      role: 'assistant',
      content,
      createdAt: Date.now()
    });

    res.status(200).send('OK');
  } catch (error) {
    console.error('Orchestrator Finalize Handler Error:', error);
    res.status(500).send('Internal Server Error');
  }
};

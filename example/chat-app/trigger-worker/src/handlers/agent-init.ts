import { Request, Response } from 'express';
import { TriggerHandler, WebhookPayload } from '@syntrix/client';
import { AgentTask, SubAgent } from '../types';
import { generateShortId } from '../utils';

export const agentInitHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received Agent Init trigger: ${payload.triggerId}`);

    const task = payload.after as AgentTask;
    if (!task || task.type !== 'agent') {
        res.status(200).send('Ignored');
        return;
    }

    if (!payload.preIssuedToken) {
      console.error('Missing preIssuedToken');
      res.status(401).send('Unauthorized');
      return;
    }

    // collection: users/demo-user/orch-chats/chat-1/tasks
    const parts = payload.collection.split('/');
    const userId = parts[1];
    const chatId = parts[3];
    const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL || 'http://localhost:8080/api/v1');
    const syntrix = handler.syntrix;

    console.log(`[AgentInit] ChatID: ${chatId}, TaskID: ${task.id}`);

    const subAgentId = generateShortId();
    const subAgent: SubAgent = {
        id: subAgentId,
        userId,
        chatId,
        taskId: task.id,
        role: task.name.split(':')[1] as any, // agent:researcher -> researcher
        status: 'active',
        createdAt: Date.now(),
        updatedAt: Date.now()
    };

    // 1. Create SubAgent
    await syntrix.doc<SubAgent>(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}`).set(subAgent);

    // 2. Update Task
    await syntrix.doc<AgentTask>(`users/${userId}/orch-chats/${chatId}/tasks/${task.id}`).update({
      subAgentId: subAgentId,
      status: 'running',
      updatedAt: Date.now()
    });

    // 3. Bootstrap Messages
    const systemPrompt = `You are a ${subAgent.role}.
    Instruction: ${task.instruction}

    CRITICAL: You MUST end every turn with either a Tool Call or a Final Answer. Do not just chat.
    If you need clarification, ask the user.`;

    // System Message
    const systemMsgId = generateShortId();
    await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${systemMsgId}`).set({
      id: systemMsgId,
      userId,
      subAgentId,
      role: 'system',
      content: systemPrompt,
      createdAt: Date.now()
    });

    // User Message (Instruction)
    const instructionMsgId = generateShortId();
    await syntrix.doc(`users/${userId}/orch-chats/${chatId}/sub-agents/${subAgentId}/messages/${instructionMsgId}`).set({
      id: instructionMsgId,
      userId,
      subAgentId,
      role: 'user',
      content: task.instruction,
      createdAt: Date.now() + 1
    });

    res.status(200).send('OK');
  } catch (error) {
    console.error('Agent Init Handler Error:', error);
    res.status(500).send('Internal Server Error');
  }
};

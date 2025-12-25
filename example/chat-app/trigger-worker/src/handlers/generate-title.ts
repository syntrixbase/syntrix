import { Request, Response } from 'express';
import { AzureOpenAI } from 'openai';
import { TriggerHandler, WebhookPayload } from '@syntrix/client';

const openai = new AzureOpenAI({
  endpoint: process.env.AZURE_OPENAI_ENDPOINT,
  apiKey: process.env.AZURE_OPENAI_API_KEY,
  apiVersion: '2024-05-01-preview',
});

export const generateTitleHandler = async (req: Request, res: Response) => {
  try {
    const payload = req.body as WebhookPayload;
    console.log(`Received Generate Title trigger: ${payload.triggerId}`);

    // 1. Parse Context
    // collection: users/demo-user/orch-chats/chat-1/messages
    const parts = payload.collection.split('/');
    if (parts.length < 5) {
      console.error('Invalid collection path:', payload.collection);
      res.status(400).send('Invalid path');
      return;
    }

    const userId = parts[1];
    const chatId = parts[3];
    console.log(`[GenerateTitle] ChatID: ${chatId}`);
    const chatPath = `users/${userId}/orch-chats/${chatId}`;
    if (!payload.preIssuedToken) {
      console.error('Missing preIssuedToken');
      res.status(401).send('Unauthorized');
      return;
    }

    const handler = new TriggerHandler(payload, process.env.SYNTRIX_API_URL || 'http://localhost:8080/api/v1');
    const syntrix = handler.syntrix;

    // 2. Check if title is already generated
    const chat = await syntrix.doc<any>(chatPath).get();
    if (chat && chat.titleGenerated) {
        console.log(`Title already generated for chat ${chatId}, skipping.`);
        res.status(200).send('Skipped');
        return;
    }

    // 3. Get content
    // We use the content of the message that triggered this event.
    const userContent = (payload.after as any)?.content;

    if (!userContent) {
        console.log('User content is empty, skipping.');
        res.status(200).send('Skipped');
        return;
    }

    // 4. Generate Title
    const completion = await openai.chat.completions.create({
      model: process.env.AZURE_OPENAI_MODEL || 'gpt-4o',
      messages: [
        { role: 'system', content: 'You are a helpful assistant. Generate a short, concise title (3-5 words) for the conversation based on the user\'s message. Do not use quotes.' },
        { role: 'user', content: userContent }
      ],
      max_tokens: 20,
    });

    const title = completion.choices[0].message.content?.trim();
    if (!title) {
        console.error('Failed to generate title');
        res.status(500).send('Failed to generate title');
        return;
    }

    console.log(`Generated title: "${title}" for chat ${chatId}`);

    // 5. Update Chat Title and set flag
    await syntrix.doc(chatPath).update({ title, titleGenerated: true });

    res.status(200).send('Title updated');

  } catch (error) {
    console.error('Error in generateTitleHandler:', error);
    res.status(500).send('Internal Server Error');
  }
};

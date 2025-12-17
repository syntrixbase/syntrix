import 'dotenv/config';
import express from 'express';
import { llmHandler } from './handlers/llm';
import { toolRunnerHandler } from './handlers/tool-runner';
import { generateTitleHandler } from './handlers/generate-title';

const app = express();
const port = process.env.PORT || 3000;

app.use(express.json());

// Routes
app.post('/webhook/llm', llmHandler);
app.post('/webhook/tool', toolRunnerHandler);
app.post('/webhook/title', generateTitleHandler);

app.get('/health', (req, res) => {
  res.send('OK');
});

app.listen(port, () => {
  console.log(`Trigger Worker listening at http://localhost:${port}`);
});

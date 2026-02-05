# Syntrix Chat App Demo

This example demonstrates an **Event-Driven LLM Agent** architecture using Syntrix. It showcases how to build a reactive chat application where business logic (LLM inference, tool execution) is decoupled from the frontend and handled by backend workers triggered by database events.

## Architecture

- **Frontend**: React + RxDB (Syncs data from Syntrix, no direct API calls to LLM).
- **Database**: Syntrix (Stores messages, tool calls, and triggers events).
- **Backend Worker**: Node.js (Listens to Syntrix Webhooks, calls Azure OpenAI / Tavily).

## Prerequisites

- **Go 1.24+** (to run Syntrix)
- **Bun** or **Node.js** (to run the worker and frontend)
- **MongoDB** (running locally or accessible)
- **NATS** (running locally for triggers)
- **Azure OpenAI API Key** (or standard OpenAI Key)
- **Tavily API Key** (for search functionality)

## Quick Start

You will need **3 separate terminal windows** to run the full stack.

### Terminal 1: Start Syntrix Server

```bash
# From the root syntrix directory
make build
./bin/syntrix --config-dir example/chat-app/config --standalone
```

This will:
- Load configuration from `example/chat-app/config/`
- Run all services in standalone mode (single process)
- API available at `http://localhost:8080`
- Realtime available at `http://localhost:8081`

### Terminal 2: Start Trigger Worker

```bash
cd example/chat-app/trigger-worker
bun install
cp .env.example .env  # Then edit with your API keys
bun run dev
```

Configure `.env`:
```env
PORT=8000
SYNTRIX_API_URL=http://localhost:8080
AZURE_OPENAI_ENDPOINT=https://your-resource.openai.azure.com/
AZURE_OPENAI_API_KEY=your_key
AZURE_OPENAI_MODEL=gpt-4o
TAVILY_API_KEY=your_tavily_key
```

### Terminal 3: Start Frontend App

```bash
cd example/chat-app
bun install
bun run dev
```

Open `http://localhost:5173` in your browser.

## Configuration

The chat-app has its own dedicated configuration in `config/`:

```
example/chat-app/config/
├── config.yml           # Main Syntrix configuration
├── keys/
│   └── auth_private.pem # JWT signing key (auto-generated)
├── security_rules/
│   └── default.yml      # Access control rules
└── triggers/
    └── default.yml      # Webhook trigger definitions
```

### Key Configuration Differences from Default

| Setting | Default | Chat-App | Why |
|---------|---------|----------|-----|
| Database | `syntrix` | `syntrix_chat` | Isolated data |
| CORS Origins | None | `localhost:5173` | Vite dev server |
| Access Token TTL | 15m | 1h | Demo convenience |
| Log Level | info | debug | Easier debugging |
| Storage Backend | Mongo + Postgres | Mongo only | Simpler setup |

### Environment Variables

You can override config values with environment variables:

```bash
# Override config directory
export SYNTRIX_CONFIG_DIR=/path/to/custom/config

# Override specific settings
export MONGO_URI=mongodb://remote:27017
export TRIGGER_NATS_URL=nats://remote:4222
```

## How to Use

1. **Sign Up**: Create an account (or use admin: `admin` / `chatapp123`)
2. **Create a Chat**: Click "+ New Chat" in the sidebar
3. **Send Messages**: Test with "Hello" or research queries
4. **Watch the Magic**:
   - Message syncs immediately (optimistic UI)
   - Trigger fires webhook to worker
   - Worker orchestrates LLM + tools
   - Results sync back to frontend

## Troubleshooting

### Syntrix won't start
```bash
# Check if MongoDB is running
mongosh --eval "db.runCommand({ping:1})"

# Check if NATS is running
nats-server --version
```

### No response from LLM
- Check Terminal 2 (Worker) logs for API errors
- Verify `.env` has correct Azure OpenAI credentials
- Check Terminal 1 (Syntrix) for trigger delivery status

### Frontend sync issues
- Open Browser Console (F12)
- Check for WebSocket connection errors
- Verify CORS settings allow `localhost:5173`

### Authentication errors
- Check `config/keys/auth_private.pem` exists
- Try regenerating: `openssl genpkey -algorithm RSA -out config/keys/auth_private.pem -pkeyopt rsa_keygen_bits:2048`

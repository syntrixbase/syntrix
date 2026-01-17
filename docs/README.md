# Syntrix Documentation

Welcome to the Syntrix developer documentation.

## Project Structure

```
syntrix/
├── cmd/
│   ├── syntrix/          # Main server binary
│   └── syntrix-cli/      # CLI tool
├── internal/             # Private application code
│   ├── config/           # Configuration loading
│   ├── core/             # Core identity/auth
│   ├── gateway/          # API Gateway (HTTP, WebSocket, SSE)
│   ├── indexer/          # Indexer service (sharded, stateful)
│   ├── puller/           # Change stream puller
│   ├── query/            # Query engine
│   ├── server/           # Server orchestration
│   ├── services/         # Service manager
│   ├── streamer/         # Real-time streamer
│   └── trigger/          # Trigger system (evaluator + delivery)
├── pkg/                  # Public library code
├── api/                  # API definitions (proto, OpenAPI)
├── tests/                # Integration tests
├── docs/                 # Documentation
│   └── design/           # Architecture and design documents
├── scripts/              # Build and development scripts
└── config/               # Configuration files
```

## Getting Started

### Prerequisites

- Go 1.24 or later
- MongoDB 4.0+ (Replica Set required for Realtime features)

### Build

```bash
make build
```

### Run

**Standalone Mode** (recommended for development):
```bash
./bin/syntrix --standalone
```
This runs all services (Gateway, Indexer, Puller, Streamer, Query, Trigger) in a single process.

**Distributed Mode**:
```bash
./bin/syntrix --all

# or
./bin/syntrix --api
# ...
```

### Configuration

Configuration is loaded in the following order:
1. Defaults
2. `config.yml`
3. `config.local.yml` (gitignored)
4. Environment Variables (`MONGO_URI`, `DB_NAME`, `API_PORT`, etc.)

### Testing

```bash
# Unit tests
make test

# Coverage analysis
make coverage
```

## Documentation Index

### Architecture & Design

- [System Architecture](architecture.md) - High-level overview
- [Design Documents](design/README.md) - Detailed component design
  - [Server Design](design/server/) - Core server components
  - [SDK Design](design/sdk/) - Client SDK architecture
  - [Monitor Design](design/monitor/) - Monitoring system

### Reference

- [REST API](reference/api.md) - HTTP API endpoints
- [Filter Syntax](reference/filters.md) - Query filter syntax
- [Trigger Rules](reference/trigger_rules.md) - Trigger configuration
- [TypeScript SDK](reference/typescript_sdk.md) - Client SDK for Node.js and Browser
- [Replication](reference/replication.md) - Data replication reference

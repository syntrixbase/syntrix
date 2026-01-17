# Syntrix

Syntrix is a Firestore-like realtime document database built with Go and MongoDB.

## What is Syntrix?

Syntrix provides a document database with real-time capabilities, allowing clients to subscribe to data changes and receive instant updates. It's designed as a **splittable monolith** - run everything in one process for simplicity, or deploy as separate microservices for scale.

## Key Features

- **Document Storage**: CRUD operations with Optimistic Locking (CAS)
- **Query Engine**: Filter and sort documents with a powerful query syntax
- **Realtime Updates**: WebSocket/SSE-based real-time events (Create, Update, Delete)
- **Triggers**: Server-side event reactions (Webhooks, Cloud Functions) with CEL conditions
- **Flexible Deployment**: Standalone mode for development, distributed mode for production

## Documentation

See [docs/README.md](docs/README.md) for detailed documentation including:
- Project structure and how to run
- Architecture and design documents
- API reference and SDK guides

## Quick Start

```bash
# Build
make build

# Run (standalone mode - all services in one process)
./bin/syntrix --standalone

# Run tests
make test
```

## License

See [LICENSE](LICENSE) for details.

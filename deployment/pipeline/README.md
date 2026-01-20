# Pipeline Infrastructure

Lightweight Docker Compose setup for CI/CD pipelines.

## Services

| Service    | Port  | Description                         |
|------------|-------|-------------------------------------|
| MongoDB    | 27017 | Document storage (replica set)      |
| PostgreSQL | 5432  | User storage                        |
| NATS       | 4222  | Message broker with JetStream       |

## Usage

```bash
# Start all services
docker compose -f deployment/pipeline/docker-compose.yml up -d

# Check status
docker compose -f deployment/pipeline/docker-compose.yml ps

# Stop all services
docker compose -f deployment/pipeline/docker-compose.yml down
```

## Connection Strings

- **MongoDB**: `mongodb://localhost:27017`
- **PostgreSQL**: `postgres://syntrix:syntrix@localhost:5432/syntrix?sslmode=disable`
- **NATS**: `nats://localhost:4222`

## Differences from Dev Environment

This setup is optimized for CI/CD:
- No persistent volumes (ephemeral storage)
- No monitoring stack (Prometheus, Grafana)
- Faster health check intervals
- Minimal resource allocation

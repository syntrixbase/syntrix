# Syntrix Development Environment

This directory contains Docker Compose configuration for running Syntrix with full monitoring stack locally.

## Quick Start

```bash
# Start all services
docker compose up -d

# View logs
docker compose logs -f

# Stop all services
docker compose down

# Stop and remove volumes (full reset)
docker compose down -v
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| MongoDB | 27017 | Primary data store (replica set) |
| NATS | 4222, 8222 | Message queue for triggers |
| Prometheus | 9090 | Metrics storage and alerting |
| Grafana | 3000 | Dashboards and visualization |

## Running Syntrix

### Option 1: Outside Docker (Recommended for Development)

```bash
# In project root
make build
./bin/syntrix --standalone

# Or with live reload
make dev
```

Prometheus is pre-configured to scrape `host.docker.internal:8080`.

### Option 2: Inside Docker

Uncomment the `syntrix` service in `docker-compose.yml`.

## Accessing Services

| Service | URL | Credentials |
|---------|-----|-------------|
| Syntrix API | http://localhost:8080 | - |
| Prometheus | http://localhost:9090 | - |
| Grafana | http://localhost:3000 | admin/admin |
| NATS Monitoring | http://localhost:8222 | - |

## Grafana Dashboards

Dashboards are auto-provisioned from `grafana/dashboards/`:

- **Syntrix Overview** - Service health, RED metrics, resource usage

## Prometheus Alerts

Alert rules are defined in `prometheus/alerts.yml`:

- Service down
- High error rate (>5%)
- High latency (P95 > 500ms, P99 > 1s)
- Puller backpressure
- High goroutine count

## Directory Structure

```
deployment/dev/
├── docker-compose.yml       # Main compose file
├── prometheus/
│   ├── prometheus.yml       # Scrape configuration
│   └── alerts.yml           # Alert rules
├── grafana/
│   ├── provisioning/
│   │   ├── datasources/     # Auto-configure Prometheus
│   │   └── dashboards/      # Auto-load dashboards
│   └── dashboards/          # Dashboard JSON files
└── scripts/
    └── mongo-init.js        # MongoDB replica set init
```

## Troubleshooting

### Prometheus can't reach Syntrix

If running Syntrix outside Docker on Linux, you may need to use the actual host IP:

```yaml
# prometheus/prometheus.yml
- targets: ["172.17.0.1:8080"]  # Docker bridge gateway
```

### MongoDB replica set not initialized

```bash
docker compose exec mongodb mongosh --eval "rs.status()"
```

If not initialized:
```bash
docker compose exec mongodb mongosh --eval "rs.initiate()"
```

### Reset everything

```bash
docker compose down -v
docker compose up -d
```

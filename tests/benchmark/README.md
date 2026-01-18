# Syntrix Benchmark Tool

A comprehensive performance testing tool for the Syntrix realtime document database.

## Quick Start

```bash
# Build the benchmark tool
make build-benchmark

# Run a basic CRUD benchmark
./bin/syntrix-benchmark run \
  --target http://localhost:8080 \
  --duration 1m \
  --workers 50 \
  --scenario crud_basic

# Run with configuration file
./bin/syntrix-benchmark run --config configs/crud_mixed.yaml

# Compare benchmark results
./bin/syntrix-benchmark compare results/run1.json results/run2.json
```

## Features

- **Multiple Scenarios**: CRUD, Query, Realtime subscriptions, and more
- **Real-time Monitoring**: Web dashboard to view live benchmark metrics
- **Detailed Metrics**: Throughput, latency percentiles, error rates
- **Flexible Configuration**: YAML config files or CLI flags
- **Live Console Output**: Real-time progress in terminal
- **Result Comparison**: Compare multiple benchmark runs
- **CI/CD Ready**: JSON output for automated regression testing

## Documentation

- [Design Document](DESIGN.md) - Architecture and implementation details
- [Task Breakdown](TASKS.md) - Development roadmap and task list
- [Monitoring Guide](docs/monitoring.md) - Real-time monitoring server documentation
- [Configuration Guide](docs/configuration.md) - Configuration options (coming soon)
- [Scenarios Guide](docs/scenarios.md) - Available benchmark scenarios (coming soon)

## Project Structure

```
tests/benchmark/
├── DESIGN.md              # Architecture and design document
├── TASKS.md              # Task breakdown and roadmap
├── README.md             # This file
├── cmd/
│   └── syntrix-benchmark/
│       └── main.go       # CLI entry point
├── pkg/
│   ├── config/          # Configuration loading and validation
│   ├── runner/          # Benchmark execution engine
│   ├── scenario/        # Benchmark scenarios
│   ├── client/          # HTTP/WebSocket client
│   ├── metrics/         # Metrics collection and reporting
│   ├── generator/       # Test data generation
│   └── utils/           # Utility functions
├── configs/             # Pre-defined scenario configurations
└── examples/            # Example custom scenarios
```

## Development Status

🚧 **Under Development** - See [TASKS.md](TASKS.md) for current progress.

### Phase 1: MVP (In Progress)
- [ ] Basic CLI framework
- [ ] CRUD scenario implementation
- [ ] Simple metrics collection
- [ ] Console output

### Phase 2: Enhanced Scenarios (Planned)
- [ ] Query scenarios
- [ ] Realtime scenarios
- [ ] YAML configuration support
- [ ] Detailed metrics and histograms

### Phase 3: Production Ready (Future)
- [ ] HTML report generation
- [ ] Benchmark comparison tool
- [ ] CI/CD integration examples
- [ ] Performance regression detection

## Contributing

See [TASKS.md](TASKS.md) for available tasks and implementation guidelines.

## License

Same as Syntrix main project.

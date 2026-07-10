# ServTrace — Distributed Tracing Backend

```bash
docker run -p 4317:4317 -p 4318:4318 ghcr.io/vyuvaraj/servtrace:latest
```

ServTrace is the centralized distributed tracing collector and visualizer backend for the Serv ecosystem. It implements OTLP/HTTP trace ingestion, allowing it to collect traces from all services and reconstruct trace waterfalls.

## Features

- **OTLP/HTTP Ingestion**: Standard `/v1/traces` ingestion endpoint.
- **Trace Reassembly**: Groups spans by trace ID, links parent-child relationships, and calculates absolute/relative duration offsets.
- **REST APIs**: Query trace summaries and waterfall hierarchy trees.
- **Eviction Policy**: Thread-safe in-memory store with oldest-first trace eviction limits.

## Getting Started

### Starting the Collector

To run the collector on port `8090`:

```bash
go run main.go --port 8090 --limit 1000
```

### APIs

- `POST /v1/traces` - Ingest OTLP/HTTP spans
- `GET /api/traces` - List trace summaries
- `GET /api/traces/{traceId}` - Fetch trace waterfall tree
- `DELETE /api/traces` - Clear all traces in memory

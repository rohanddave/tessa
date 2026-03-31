# tessa-rag

This repository is set up as a multi-service workspace with shared local infrastructure.

## Current layout

- `docker-compose.yml`: central orchestration for shared dependencies and service containers
- `services/repo-sync-service`: first service, written in Go

## Shared local infrastructure

- Kafka: shared event bus for all services
- Kafka UI: local topic and consumer inspection
- MinIO: local S3-compatible object storage

## Quick start

```bash
docker compose up --build
```

Once the stack is up:

- `repo-sync-service` is available at `http://localhost:8081`
- health check is at `http://localhost:8081/healthz`
- Kafka UI is available at `http://localhost:8080`
- MinIO API is available at `http://localhost:9000`
- MinIO console is available at `http://localhost:9001`

Default local MinIO credentials:

- username: `minioadmin`
- password: `minioadmin`

The `repo-sync` bucket is created automatically during startup.


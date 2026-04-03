# repo-sync-service

Starter Go service for repository synchronization workflows.

## Responsibilities

- receive repo sync requests through an API process
- publish sync events to Kafka
- support decoupled consumer processes for background work
- persist sync artifacts or snapshots to S3-compatible storage via MinIO locally
- interact with the shared Snapshot Store Postgres database using `pgx`

## Structure

```text
cmd/
  api/
    main.go
  consumer/
    main.go
internal/
  config/
  http/
  kafka/
  sync/
```

## API

### `POST /register-repo-event`

Registers a dummy repo event and returns the accepted event payload.

Example request:

```json
{
  "repo_url": "https://github.com/example/repo.git",
  "provider": "github",
  "branch": "main",
  "event_type": "repo.created",
  "requested_by": "tessa-user"
}
```

Validation rules:

- `repo_url` is required
- `provider` must be one of `github`, `gitlab`, or `bitbucket`
- `event_type` must be one of `repo.created`, `repo.updated`, or `repo.deleted`
- `requested_by` is required
- `branch` is optional and defaults to `main`

## Persistence

The service uses `github.com/jackc/pgx/v5` with `pgxpool` for PostgreSQL access. This keeps database access lightweight and explicit without introducing a full ORM.

For object storage, the service uses `github.com/minio/minio-go/v7` to interact with the shared MinIO-backed file store through an S3-compatible API.

## Local run

This service is intended to run through the root compose stack:

```bash
docker compose up --build repo-sync-service-api repo-sync-service-consumer
```

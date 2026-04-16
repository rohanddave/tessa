# tessa-rag

This repository is a multi-service workspace with one root Docker Compose stack and shared local infrastructure. The first service in the repo is `repo-sync-service`, a Go service that accepts repo lifecycle events, publishes them to Kafka, and processes them through a decoupled consumer.

## Current project state

The currently implemented end-to-end ingestion/indexing slice is:

1. An HTTP client sends `POST /register-repo-event` to `repo-sync-service-api`.
2. The API validates the request and converts it into a `RepoEvent`.
3. The API publishes the event to Kafka and waits for delivery confirmation.
4. `repo-sync-service-consumer` reads the event from Kafka.
5. For `repo.created`, the consumer invokes `RegisterRepoService`.
6. `RegisterRepoService` atomically marks the repo as `registering` through the repo registry in Postgres.
7. The GitHub data source downloads the repository archive and streams files one by one.
8. Files are uploaded to MinIO.
9. A manifest is created and uploaded to MinIO.
10. A snapshot record is stored in Postgres.
11. The repo is marked `registered` in the Postgres-backed repo registry.
12. The Kafka message is committed only after successful processing.

`repo.updated` and `repo.deleted` event routes also exist now. `repo.updated` has a working manifest-diff flow, while `repo.deleted` remains simpler and focused on cleanup.

## Repository layout

- `docker-compose.yml`: root orchestration for infrastructure and service containers
- `services/repo-sync-service`: first application service, written in Go
- `services/query-service`: Python query-time service for retrieval, context assembly, and LLM answering

## Shared local infrastructure

- Kafka: shared event bus for all services
- Kafka UI: local Kafka inspection UI
- MinIO: local S3-compatible object storage
- Snapshot Store: shared PostgreSQL instance for repo state and snapshot metadata

## Quick start

```bash
docker compose up --build
```

Once the stack is up:

- `repo-sync-service-api` is available at `http://localhost:8081`
- health check is at `http://localhost:8081/healthz`
- `query-service` is available at `http://localhost:8082`
- query service health check is at `http://localhost:8082/healthz`
- Kafka UI is available at `http://localhost:8080`
- MinIO API is available at `http://localhost:9000`
- MinIO console is available at `http://localhost:9001`
- Snapshot Store Postgres is available at `localhost:5432`

Default local MinIO credentials:

- username: `minioadmin`
- password: `minioadmin`

Default local Snapshot Store credentials:

- database: `snapshot_store`
- username: `postgres`
- password: `postgres`

The `repo-sync` bucket is created automatically during startup.

## Docker Compose stack

The root [docker-compose.yml](/Users/rohandave/Documents/Projects/tessa-rag/docker-compose.yml) defines the shared runtime environment for the whole repo.

### Kafka

Kafka is the shared event bus. It runs in single-node KRaft mode with no ZooKeeper and is reachable:

- inside Docker at `kafka:9092`
- from the host at `localhost:19092`

The current topic model used by `repo-sync-service` is:

- `repo-sync.repo-lifecycle`
  used for lifecycle-style events like `repo.created` and `repo.deleted`
- `repo-sync.repo-events`
  used for content/update-style events like `repo.updated`

### Kafka UI

Kafka UI is available at `http://localhost:8080` and points at `kafka:9092`.

### MinIO

MinIO provides the S3-compatible blob store used by `repo-sync-service`.

Key details:

- API endpoint: `http://localhost:9000`
- console: `http://localhost:9001`
- bucket used by the service: `repo-sync`

### MinIO init job

`minio-init` is a one-time setup container that waits for MinIO, creates the `repo-sync` bucket, and ensures it exists before the application containers start.

### Snapshot Store

`snapshot-store` is a shared PostgreSQL 16 instance.

It currently stores:

- repo registration state through the Postgres `RepoRegistryRepo`
- snapshot metadata through the Postgres `SnapshotStoreRepo`

### repo-sync-service-api

This container runs the API binary built from `services/repo-sync-service`.

It is responsible for:

- accepting `RepoEvent` requests over HTTP
- validating them
- publishing them to Kafka

### repo-sync-service-consumer

This container runs the consumer binary built from the same Go service image.

It is responsible for:

- subscribing to Kafka topics
- decoding Kafka messages into `RepoEvent`
- running business workflows like repo registration
- committing Kafka messages only after successful handling

## repo-sync-service architecture

The service is intentionally split into separate runtimes:

```text
services/repo-sync-service/
  cmd/
    api/
      main.go
    consumer/
      main.go
  internal/
    config/
    github/
    http/
    kafka/
    minio/
    postgres/
    sync/
    util/
```

### Runtime entrypoints

- [cmd/api/main.go](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/cmd/api/main.go)
  starts the HTTP API server
- [cmd/consumer/main.go](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/cmd/consumer/main.go)
  starts Kafka consumers and dispatches workflow handling

### Internal packages

- [internal/config](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/config)
  loads environment configuration for Kafka, GitHub, Postgres, and MinIO
- [internal/http](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/http)
  HTTP routes and request handling
- [internal/kafka](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/kafka)
  Kafka producer/consumer adapter layer
- [internal/github](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/github)
  GitHub-backed repo data source implementation
- [internal/minio](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/minio)
  MinIO-backed blob store implementation
- [internal/postgres](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/postgres)
  Postgres-backed snapshot store and repo registry implementations
- [internal/sync/domain](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/sync/domain)
  domain models like `Manifest` and `Snapshot`
- [internal/sync/ports](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/sync/ports)
  outbound interfaces for data source, blob store, snapshot store, and repo registry
- [internal/sync/service](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/sync/service)
  business workflows like repo registration, repo update, and deletion
- [internal/util](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/util)
  small shared helpers like env loading and content hashing

## API flow

The main API endpoint is:

- `POST /register-repo-event`

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
- `branch` defaults to `main`

What happens inside the API:

1. The request is decoded into `RepoEventRequest`.
2. Validation is performed.
3. A `RepoEvent` is built.
4. The Kafka producer routes the event:
   - `repo.created` and `repo.deleted` -> lifecycle topic
   - `repo.updated` -> events topic
5. The producer waits for Kafka delivery confirmation.
6. On success, the API returns `202 Accepted` with the event payload.

## Kafka consumer flow

The Kafka consumer layer uses an internal wrapper message type so the consumer runtime can commit Kafka offsets without leaking Confluent Kafka types into the service layer.

Current behavior:

- Kafka auto-commit is disabled
- the consumer reads a wrapped message
- the wrapped message exposes only the decoded `RepoEvent`
- the business handler runs
- if handling succeeds, the consumer commits the Kafka message
- if handling fails, the message is not committed

This keeps Kafka details inside [internal/kafka](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/kafka).

## `repo.created` registration flow

The `repo.created` handling path is the most complete business workflow in the project right now.

### Step 1: state transition

`RegisterRepoService` calls the Postgres-backed repo registry:

- `TryStartRegistration(repoURL, branch, commitSHA)`

This is implemented as an atomic SQL transition, not an in-memory mutex. It succeeds only if the repo is eligible to enter the `registering` state.

### Step 2: repo archive streaming

The GitHub data source implementation:

- parses the GitHub repo URL
- downloads the GitHub tar archive for the requested branch/ref
- unwraps the archive stream internally
- exposes repo files one at a time through the `DataSourceRepo` port

The service does not parse tar directly. It consumes file-level records through the `RepoFileStream` abstraction.

### Step 3: worker pipeline

`RegisterRepoService`:

- reads files from the data source stream
- buffers them into file jobs
- fans those jobs out to worker goroutines
- hashes file contents
- uploads files to MinIO through `BlobStoreRepo`
- appends manifest entries under a mutex

### Step 4: manifest creation

After all workers complete:

- the manifest is marshaled to JSON
- the manifest is stored in MinIO as `manifest.json` under the repo prefix

### Step 5: snapshot persistence

Then the service stores a `Snapshot` row in Postgres using `SnapshotStoreRepo`.

The snapshot currently records:

- repo URL
- branch
- commit SHA
- manifest URL

### Step 6: final state transition

Finally, the repo registry marks the repo as:

- `registered`

Only after that does the consumer commit the Kafka message.

## `repo.updated` update flow

`repo.updated` is now modeled as an incremental snapshot refresh for the single tracked branch of a repo.

### Step 1: repo and branch eligibility

`RepoUpdateService` first checks the repo registry:

- `TryUpdateRepo(repoURL, branch)`

This ensures updates only proceed for repos that are already registered and only for the tracked branch stored in the repo registry.

### Step 2: previous snapshot lookup

The update flow loads the latest snapshot for the repo from `SnapshotStoreRepo`.

If there is no previous snapshot, the update is rejected because there is no baseline to diff against.

### Step 3: previous manifest load

The service fetches the previous manifest from MinIO using the manifest URL stored in the previous snapshot.

That manifest is unmarshaled and copied into a new in-memory working manifest for the new commit.

### Step 4: archive streaming and file comparison

The GitHub data source streams the new repo archive for the incoming commit.

As files are streamed:

- each file is hashed
- the path is marked as seen for the current update run
- if the previous hash for that path matches, the file is treated as unchanged
- if the path is new or its hash changed, the blob is uploaded to MinIO and the manifest entry is updated

### Step 5: deleted file detection

After the stream finishes, any file path that existed in the previous manifest but was not seen in the new stream is removed from the working manifest.

This is how deleted files are detected without requiring a second archive or a Git-specific diff API.

### Step 6: new manifest and snapshot

Once the diff is complete:

- the new manifest is written to MinIO
- a new snapshot row is created in Postgres

### Step 7: registry update completion

Finally, the repo registry calls:

- `MarkUpdated(repoURL, commitSHA)`

This moves the repo from `updating` back to `registered` and stores the latest commit SHA in the repo registry record.

Only after that does the consumer commit the Kafka message.

## Current storage adapters

### Blob store

The blob store is implemented by [internal/minio/blob_store_repo.go](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/minio/blob_store_repo.go).

It currently supports:

- `InsertFile`
- `GetFile`
- `RemoveFile`
- `RemoveDirectory`

`RemoveDirectory` deletes all objects under a prefix, which is intended for repo cleanup flows.

### Snapshot store

The snapshot store is implemented by [internal/postgres/snapshot_store_repo.go](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/postgres/snapshot_store_repo.go).

It currently supports:

- `CreateSnapshot`
- `GetSnapshot`
- `GetLatestSnapshot`
- `DeleteSnapshotsByRepoURL`

### Repo registry

The repo registry is implemented by [internal/postgres/repo_registry_repo.go](/Users/rohandave/Documents/Projects/tessa-rag/services/repo-sync-service/internal/postgres/repo_registry_repo.go).

It currently supports:

- `TryStartRegistration`
- `MarkRegistered`
- `TryStartDeletion`
- `MarkDeleted`
- `TryUpdateRepo`
- `MarkUpdated`

These methods are implemented with atomic SQL transitions so they work across multiple service instances and containers.

## Current dependencies

The service currently uses:

- `confluent-kafka-go/v2` for Kafka
- `pgx/v5` and `pgxpool` for Postgres
- `minio-go/v7` for blob storage

## Environment configuration

The service currently relies on these main env vars:

### Kafka

- `KAFKA_BROKERS`
- `KAFKA_EVENTS_TOPIC`
- `KAFKA_LIFE_CYCLE_TOPIC`

### GitHub

- `GITHUB_TOKEN`

### Postgres

- `SNAPSHOT_STORE_HOST`
- `SNAPSHOT_STORE_PORT`
- `SNAPSHOT_STORE_DB`
- `SNAPSHOT_STORE_USER`
- `SNAPSHOT_STORE_PASSWORD`
- `SNAPSHOT_STORE_SSLMODE`

### MinIO

- `S3_ENDPOINT`
- `S3_REGION`
- `S3_BUCKET`
- `S3_ACCESS_KEY_ID`
- `S3_SECRET_ACCESS_KEY`
- `S3_USE_SSL`

## Startup order

When you run `docker compose up --build`:

1. Kafka starts and becomes healthy.
2. MinIO starts.
3. `minio-init` creates the `repo-sync` bucket.
4. `snapshot-store` starts and initializes the shared Postgres database.
5. `repo-sync-service-api` starts.
6. `repo-sync-service-consumer` starts.

## Future scope

There are a few known areas where the current implementation works functionally but should be improved for production resilience.

### Bound memory usage for file jobs

The current registration and update pipelines read each streamed file fully into memory before it is handed to worker goroutines through `FileJob`.

Future work:

- bound the `fileJobs` channel size
- cap the maximum in-memory bytes allowed across queued jobs
- consider spilling large files to temporary storage or streaming directly into blob-store uploads

This will keep large repos from causing uncontrolled memory growth.

### Make concurrency configurable

The current worker counts are hard-coded in the services and consumer runtime.

Future work:

- expose worker counts through environment variables
- separately tune:
  - Kafka consumer goroutine count
  - registration file worker count
  - update file worker count

This will make the service easier to tune for local development versus larger deployments.

### Support explicit skip-but-commit consumer outcomes

Right now the consumer commits Kafka messages only after a handler returns success, and leaves messages uncommitted on error.

There are cases where a consumer may intentionally not perform the full business workflow but should still commit the message, for example:

- duplicate or already-applied updates
- events for untracked branches
- events that are valid but intentionally no-op

Future work:

- model handler outcomes more explicitly than just `error` or `nil`
- support outcomes like:
  - processed and commit
  - skipped and commit
  - retry later and do not commit
  - dead-letter then commit

That will make consumer behavior more precise and prevent retry loops for events that should be safely acknowledged.
7. Kafka UI starts and connects to Kafka.

## Notes

- The codebase is still evolving quickly, so some flows are more complete than others.
- `repo.created` is the primary path that has been wired through the current architecture.
- The README is meant to describe the current implementation, not a final target architecture.

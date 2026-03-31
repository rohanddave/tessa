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

- `repo-sync-service-api` is available at `http://localhost:8081`
- health check is at `http://localhost:8081/healthz`
- `repo-sync-service-consumer` runs as a separate background container
- Kafka UI is available at `http://localhost:8080`
- MinIO API is available at `http://localhost:9000`
- MinIO console is available at `http://localhost:9001`

Default local MinIO credentials:

- username: `minioadmin`
- password: `minioadmin`

The `repo-sync` bucket is created automatically during startup.

## Docker Compose breakdown

The root [`docker-compose.yml`](/Users/rohandave/Documents/Projects/tessa-rag/docker-compose.yml) is the central orchestration file for this repository. Shared infrastructure lives here once, and each service plugs into it.

### Kafka

The `kafka` service is the shared event bus for all services in this repo. It uses the official `apache/kafka:3.7.2` image and runs in KRaft mode, so no ZooKeeper is required.

Key pieces:

- `ports`: exposes `9092` for container-to-container traffic and `19092` for clients running on the host machine
- `hostname: kafka`: gives other containers a stable internal hostname
- `environment`: defines a single-node Kafka cluster
- `CLUSTER_ID`: identifies the KRaft cluster
- `KAFKA_NODE_ID`: identifies this node
- `KAFKA_PROCESS_ROLES: broker,controller`: makes this container act as both broker and controller
- `KAFKA_LISTENERS`: tells Kafka which ports to listen on
- `KAFKA_ADVERTISED_LISTENERS`: tells clients what addresses to use
- `PLAINTEXT://kafka:9092`: internal address for other containers
- `EXTERNAL://localhost:19092`: host address for local tools on the machine
- replication-related values are all set to `1` because this is a local single-node setup
- `volumes`: `kafka-data` persists broker data across restarts
- `healthcheck`: uses `kafka-topics.sh` to verify the broker is actually ready before dependents start

### Kafka UI

The `kafka-ui` service provides a browser UI for inspecting topics, messages, and consumers.

Key pieces:

- `depends_on`: waits until Kafka is healthy
- `ports`: exposes the UI at `http://localhost:8080`
- `environment`: points the UI to the broker at `kafka:9092` inside the Docker network

### MinIO

The `minio` service provides a local S3-compatible object store so services can develop against an S3-style API without using AWS in local development.

Key pieces:

- `command: server /data --console-address ":9001"`: starts MinIO and enables the admin console
- `ports`: `9000` for the S3-compatible API and `9001` for the web console
- `environment`: sets local development credentials to `minioadmin` / `minioadmin`
- `volumes`: `minio-data` persists buckets and objects across restarts

### MinIO init job

The `minio-init` service is a short-lived setup job that prepares the bucket expected by `repo-sync-service`.

Key pieces:

- `image: minio/mc:latest`: uses the MinIO command-line client
- `depends_on`: starts after MinIO has started
- `entrypoint`: retries until MinIO is reachable, then creates the `repo-sync` bucket if needed and sets it to private
- `restart: "no"`: this is intentional because it is a one-time initialization task

### repo-sync-service-api

The `repo-sync-service-api` service is the API runtime for the first application service in the repo. Docker builds it from the local Go code in `./services/repo-sync-service`.

Key pieces:

- `build`: builds the shared image from the Go service directory
- `depends_on`: waits for Kafka, MinIO, and the MinIO init job
- `environment`: injects runtime configuration into the app
- `PORT`, `SERVICE_NAME`, `LOG_LEVEL`: service-level settings
- `KAFKA_BROKERS`, `KAFKA_TOPIC`: Kafka connection settings
- `S3_*`: MinIO settings exposed in an S3-compatible shape
- `ports`: exposes the service at `http://localhost:8081`

### repo-sync-service-consumer

The `repo-sync-service-consumer` service uses the same Docker image as the API, but starts the consumer binary instead of the API binary. This keeps background Kafka consumption decoupled from request handling.

Key pieces:

- reuses the same build and environment as the API container
- starts the `repo-sync-service-consumer` binary
- has no public port because it is a background worker
- can scale independently from the API in the future

### Networks

The `networks` section defines `tessa-net`, a shared bridge network. Every service joins this network so they can resolve one another by service name, such as `kafka` and `minio`.

### Volumes

The `volumes` section defines named persistent storage:

- `kafka-data`: stores Kafka state between restarts
- `minio-data`: stores MinIO buckets and objects between restarts

### Startup order

When you run `docker compose up --build`:

1. Kafka starts and becomes healthy.
2. MinIO starts.
3. `minio-init` creates the `repo-sync` bucket.
4. `repo-sync-service-api` starts with Kafka and S3-compatible config.
5. `repo-sync-service-consumer` starts as a separate background process.
6. `kafka-ui` starts and connects to Kafka.

# tessa-rag

Tessa RAG is a local multi-service code retrieval system. It ingests repositories, chunks source files with Tree-sitter, indexes chunks into text, vector, and graph stores, and serves retrieval/answering requests through a Python query service.

The project is designed around one root Docker Compose stack and a shared event-driven pipeline:

```text
repo event
  -> repo-sync-service
  -> MinIO + Postgres snapshot
  -> chunking-service
  -> Postgres chunks + Kafka indexing events
  -> indexing-service
  -> Elasticsearch + Pinecone local index + Neo4j
  -> query-service
  -> retrieval, fusion, context assembly, answer generation
```

## Repository Layout

```text
services/
  repo-sync-service/     Go service for repo registration, update, deletion, snapshots
  chunking-service/      Go service for Tree-sitter extraction and chunk creation
  indexing-service/      Go service for text, vector, and graph indexing
  query-service/         Python service for retrieval, context assembly, and answers
  answer-eval-service/   Python runner for comparing answer strategies
  shared/                Shared Go domain, Kafka, Postgres, blob store, utilities

docker-compose.yml       Local infrastructure and service orchestration
README.md                Project overview
```

## Local Infrastructure

The root `docker-compose.yml` starts:

| Component | Purpose | Host |
|---|---|---|
| Kafka | Event bus | `localhost:19092` |
| Kafka UI | Kafka inspection | `http://localhost:8080` |
| MinIO | S3-compatible repo blob store | `http://localhost:9000` |
| MinIO Console | Blob store UI | `http://localhost:9001` |
| Postgres | Snapshot, registry, chunk metadata/content | `localhost:5432` |
| Elasticsearch | Keyword/text index | `http://localhost:9200` |
| Neo4j | Code relationship graph | `http://localhost:7474`, `bolt://localhost:7687` |
| Pinecone Local | Vector index | `http://localhost:5081` |
| repo-sync API | Repo lifecycle API | `http://localhost:8081` |
| query service | Retrieval and answer API | `http://localhost:8082` |
| answer eval service | One-shot answer strategy evaluation runner | `docker compose --profile eval run --rm answer-eval-service` |

Default local credentials:

```text
MinIO:
  username: minioadmin
  password: minioadmin

Postgres:
  database: snapshot_store
  username: postgres
  password: postgres

Neo4j:
  username: neo4j
  password: password
```

## Quick Start

Start the full stack:

```bash
docker compose up --build
```

Health checks:

```bash
curl http://localhost:8081/healthz
curl http://localhost:8082/healthz
```

Register a repository:

```bash
curl -X POST http://localhost:8081/register-repo-event \
  -H 'Content-Type: application/json' \
  -d '{
    "repo_url": "https://github.com/example/repo.git",
    "provider": "github",
    "branch": "main",
    "event_type": "repo.created",
    "requested_by": "local"
  }'
```

Retrieve code context:

```bash
curl -X POST http://localhost:8082/retrieve \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "where is InvoiceService used?",
    "repo_url": "https://github.com/example/repo.git",
    "branch": "main",
    "top_k": 8
  }'
```

Ask for an answer:

```bash
curl -X POST http://localhost:8082/answer \
  -H 'Content-Type: application/json' \
  -d '{
    "query": "explain the invoice creation flow",
    "repo_url": "https://github.com/example/repo.git",
    "branch": "main",
    "top_k": 8,
    "context_token_budget": 12000
  }'
```

Compare answer strategies:

```bash
docker compose --profile eval run --rm answer-eval-service
```

The evaluation runner calls `/answer` for the default C++ question set across `baseline`, `reasoning`, and `agentic`, then writes CSV metrics, raw JSONL responses, and matplotlib PNG plots under `eval-results/`, including aggregate per-answer token usage charts.

## End-To-End Data Flow

### 1. Repo Sync

`repo-sync-service` accepts repo lifecycle events through `POST /register-repo-event`.

Supported event types:

```text
repo.created
repo.updated
repo.deleted
```

For `repo.created`, the service:

1. Validates and publishes the event to Kafka.
2. Streams the repository archive from GitHub.
3. Uploads file blobs to MinIO.
4. Builds and stores a manifest in MinIO.
5. Stores a snapshot row in Postgres.
6. Publishes a snapshot-created event.

For `repo.updated`, it compares the new archive against the previous manifest and creates a new snapshot with changed/deleted file handling.

For `repo.deleted`, it removes stored repo state and blobs.

### 2. Chunking

`chunking-service` consumes snapshot events, loads files from MinIO, parses code with Tree-sitter, and writes chunks to Postgres.

Each chunk contains:

```text
chunk_id
repo_url
branch
commit_sha
snapshot_id
file_path
language
content
content_hash
symbol_name
symbol_type
start_line / end_line
start_char / end_char
prev_chunk_id / next_chunk_id
indexing statuses
created_at
```

Supported parser languages:

```text
Go
Java
JavaScript
TypeScript
Python
C/C++
```

The current chunk symbol types include:

```text
file
doc_comment
import
class
method
module
symbol:class
symbol:method
symbol:module
container:class
container:method
```

`symbol:*` and `container:*` types are normalized by graph indexing into canonical types such as `class`, `method`, and `module`.

### 3. Indexing

`indexing-service` consumes chunk indexing events and indexes each chunk into three retrieval backends:

| Backend | Purpose |
|---|---|
| Elasticsearch | Keyword and lexical search over chunk content and metadata |
| Pinecone Local | Vector search using OpenAI embeddings |
| Neo4j | Chunk-level code relationship graph |

Indexing is per chunk and idempotent. The indexing service marks per-backend status in Postgres:

```text
elasticsearch_status
pinecone_status
neo4j_status
```

## Neo4j Knowledge Graph

The graph is chunk-centric: every graph vertex is a `:Chunk`.

Real code chunks and synthetic file chunks share the same label:

```text
(:Chunk {symbol_type: "file", synthetic: true})
(:Chunk {symbol_type: "class", synthetic: false})
(:Chunk {symbol_type: "method", synthetic: false})
(:Chunk {symbol_type: "import", synthetic: false})
```

All graph nodes are scoped by repository/snapshot metadata:

```text
repo_url
branch
commit_sha
snapshot_id
file_path
```

Current graph relationships:

| Relationship | Meaning |
|---|---|
| `CONTAINS` | File contains chunk, or class contains method |
| `PREV` / `NEXT` | Linear chunk neighbors |
| `HAS_IMPORT` | File has import chunk |
| `RESOLVES_TO_FILE` | Import resolves to a synthetic file chunk when possible |
| `DOCUMENTS` | Doc comment documents nearest declaration |
| `ALIAS_OF` | Duplicate symbol/container chunks represent same declaration range |

The graph indexer creates placeholder nodes for prev/next links if neighbors arrive later, then hydrates those nodes when their chunk event is indexed.

Useful Neo4j checks:

```cypher
MATCH (c:Chunk)
RETURN count(c) AS chunks;
```

```cypher
MATCH ()-[r]->()
RETURN type(r) AS relationship, count(*) AS count
ORDER BY count DESC;
```

```cypher
MATCH (c:Chunk)
RETURN c.repo_url, c.snapshot_id, c.symbol_type, count(*) AS chunks
ORDER BY chunks DESC;
```

```cypher
MATCH p = (:Chunk)-[:CONTAINS|PREV|NEXT|HAS_IMPORT|RESOLVES_TO_FILE|DOCUMENTS|ALIAS_OF]->(:Chunk)
RETURN p
LIMIT 50;
```

## Query Service

`query-service` is the Python query-time API. It owns:

1. Answer strategy selection
2. Query understanding when the selected strategy needs it
3. Retrieval orchestration
4. Retriever execution
5. Fusion
6. Context expansion
7. Context assembly and budget management
8. LLM answer generation

Endpoints:

```text
GET  /healthz
POST /retrieve
POST /answer
```

### Request Model

`/retrieve` and `/answer` accept a request shaped like:

```json
{
  "query": "where is InvoiceService used?",
  "repo_url": "https://github.com/example/repo.git",
  "branch": "main",
  "snapshot_id": "optional-specific-snapshot",
  "top_k": 8,
  "context_token_budget": 12000,
  "mode": "baseline"
}
```

`repo_url` and `snapshot_id` are optional, but using them makes retrieval much more precise. `branch` defaults to `main`.

`mode` is used by `/answer` and defaults to `baseline`. Supported answer modes are:

```text
baseline
reasoning
agentic
```

`/retrieve` ignores `mode` and runs the retrieval orchestrator directly.

### Answer Strategies

The query service supports three answer strategies. They differ in how much reasoning happens before and between retrieval calls.

#### Baseline

Baseline is the direct path.

```text
original query -> retrieve -> answer
```

It does not run LLM query understanding or subquery planning. The original query is passed directly into retrieval, then the retrieved context is assembled and answered.

#### Reasoning

Reasoning does one planning pass before retrieval.

```text
original query understanding
-> subquery generation
-> per-subquery retrieval planning
-> retrieve
-> fuse
-> answer
```

It first understands the original query, generates a bounded list of retrieval-focused subqueries, creates a retrieval plan for each subquery, retrieves chunks for those subqueries, fuses the combined results, expands context, and answers from the assembled context.

#### Agentic

Agentic plans retrieval step by step.

```text
original query understanding
-> iterative next-step subquery generation
-> per-step retrieval planning
-> retrieve
-> observe
-> repeat
-> fuse
-> answer
```

It starts with the original query understanding, asks for one next retrieval step at a time, retrieves with that step's retrieval plan, summarizes the observed hits, and feeds those observations into the next planning step. The loop stops when the planner decides enough context has been observed or the step limit is reached.

### Query Understanding

`QueryUnderstandingService` extracts:

```text
intent
entities
retrieval_plan
```

Entities include backticked symbols and file-like paths. When entities or dependency-style words are present, graph retrieval is included in the retrieval plan.

Default plan:

```text
keyword + vector
```

Graph-aware plan:

```text
keyword + vector + graph
```

### Retriever Backends

The query service has three retrievers:

| Retriever | Backend | Role |
|---|---|---|
| `KeywordRetriever` | Elasticsearch | Lexical search over content, file paths, symbols, language |
| `VectorRetriever` | OpenAI embeddings + Pinecone Local | Semantic search |
| `GraphRetriever` | Neo4j + Postgres | Code relationship traversal and chunk hydration |

### Keyword Retrieval

Keyword retrieval queries Elasticsearch with a multi-match query over:

```text
content
symbol_name
file_path
symbol_type
language
```

It applies request scope filters for:

```text
repo_url
branch
snapshot_id
```

### Vector Retrieval

Vector retrieval:

1. Embeds the user query with OpenAI.
2. Queries Pinecone Local.
3. Requests metadata in the vector result.
4. Maps returned metadata into `RetrievalHit`.

Vector filters use the same request scope:

```text
repo_url
branch
snapshot_id
```

### Graph Retrieval

Graph retrieval is split into two steps:

```text
Neo4j discovers relevant chunk IDs and graph scores.
Postgres hydrates those chunk IDs into full chunk content/metadata.
```

Neo4j is used for traversal because it knows relationships such as:

```text
CONTAINS
PREV
NEXT
HAS_IMPORT
RESOLVES_TO_FILE
DOCUMENTS
ALIAS_OF
```

Postgres is used for hydration because it is the source of truth for chunk content.

Graph retrieval scores come from Neo4j traversal logic. Postgres `mget_chunks` returns chunks by ID and does not calculate relevance. ID lookup is treated as hydration, not search.

### Fusion

`FusionRanker` combines retriever outputs with reciprocal-rank-style scoring:

```text
score contribution = 1 / (60 + rank)
```

When multiple retrievers return the same `chunk_id`, fusion:

1. Merges source labels.
2. Keeps the maximum retriever score.
3. Fills missing metadata/content from later duplicate hits.

This matters because graph hits can arrive as metadata-only graph results first, then be hydrated by Postgres or duplicated by keyword/vector with full content.

### Context Expansion

Context expansion adds local neighboring chunks after fusion.

The expansion service:

1. Reads `prev_chunk_id` and `next_chunk_id` from fused hits.
2. Fetches those IDs from Postgres with `mget_chunks`.
3. Filters neighbors to the request scope.
4. Merges neighbors in a stable order:

```text
prev -> current -> next
```

Like graph hydration, context expansion does not ask Postgres for a relevance score. The neighbor is included because the primary chunk was retrieved and the neighbor is structurally adjacent.

### Context Assembly

`ContextAssemblyService` converts retrieval hits into context blocks for answer generation.

It:

1. Deduplicates hits by `chunk_id`.
2. Skips hits with empty content.
3. Estimates tokens as `len(text) // 4`, minimum `1`.
4. Adds blocks until `context_token_budget` is reached.
5. Logs budget usage, skipped hits, and source counts.

The resulting `AssembledContext` contains:

```text
repo_url
branch
snapshot_id
query
token_budget
blocks[]
```

Each block includes:

```text
chunk_id
file_path
language
symbol_name
start_line
end_line
content
score
sources
```

### Answer Generation

`LLMAnsweringService` uses assembled context to call the configured OpenAI answer model.

`/answer` responses include `token_usage`:

```text
input_tokens
output_tokens
total_tokens
```

For strategy modes, `token_usage` is aggregated across the full answer request, including query-understanding, planning, and final-answer LLM calls.

Default answer configuration:

```text
OPENAI_ANSWER_MODEL=gpt-5.4-mini
OPENAI_ANSWER_MAX_OUTPUT_TOKENS=1200
OPENAI_TIMEOUT_SECONDS=60
```

### Query Service Storage Responsibilities

| Store | Query-time responsibility |
|---|---|
| Elasticsearch | Keyword search |
| Pinecone Local | Vector search |
| Neo4j | Graph traversal and relationship-aware discovery |
| Postgres | Chunk hydration and prev/next context expansion |
| OpenAI | Query embeddings and final answer generation |

## Observability

The services log important pipeline stats.

### Indexing Service Logs

The Neo4j graph indexer logs:

```text
chunk_id
repo_url
branch
snapshot_id
file_path
symbol_name
symbol_type
prev/next presence
import candidate count
nodes created/deleted
relationships created/deleted
properties set
labels added
duration_ms
```

### Query Service Logs

The query service logs:

```text
retrieval start/end
selected retrievers
query intent
entity count
per-retriever hit counts
per-retriever source counts
fusion input/output counts
context expansion neighbor counts
context assembly block counts
empty-content skips
budget skips
estimated budget used/remaining
source counts
duration_ms
```

The logs intentionally avoid dumping chunk content.

## Environment Variables

### Shared Postgres

```text
SNAPSHOT_STORE_HOST
SNAPSHOT_STORE_PORT
SNAPSHOT_STORE_DB
SNAPSHOT_STORE_USER
SNAPSHOT_STORE_PASSWORD
SNAPSHOT_STORE_SSLMODE
```

### Kafka

```text
KAFKA_BROKERS
KAFKA_EVENTS_TOPIC
KAFKA_SNAPSHOTS_TOPIC
KAFKA_INDEXING_TOPIC
```

### MinIO / S3

```text
S3_ENDPOINT
S3_REGION
S3_BUCKET
S3_ACCESS_KEY_ID
S3_SECRET_ACCESS_KEY
S3_USE_SSL
```

### Elasticsearch

```text
ELASTICSEARCH_URL
ELASTICSEARCH_INDEX
```

### Pinecone Local

```text
PINECONE_HOST
PINECONE_API_KEY
PINECONE_API_VERSION
PINECONE_INDEX
PINECONE_NAMESPACE
```

### Neo4j

```text
NEO4J_URI
NEO4J_USER
NEO4J_PASSWORD
```

### OpenAI

```text
OPENAI_API_KEY
OPENAI_BASE_URL
OPENAI_EMBEDDING_MODEL
OPENAI_EMBEDDING_DIMENSIONS
OPENAI_ANSWER_MODEL
OPENAI_ANSWER_MAX_OUTPUT_TOKENS
OPENAI_TIMEOUT_SECONDS
```

## Development Commands

Run Go tests:

```bash
cd services/chunking-service && go test ./...
cd services/indexing-service && go test ./...
cd services/repo-sync-service && go test ./...
```

Run query-service syntax check:

```bash
cd services/query-service
python -m compileall app tests
```

Run query-service tests when dev dependencies are installed:

```bash
cd services/query-service
python -m pytest -q
```

Inspect Postgres chunk indexing status:

```sql
SELECT
  elasticsearch_status,
  pinecone_status,
  neo4j_status,
  count(*)
FROM chunks
GROUP BY elasticsearch_status, pinecone_status, neo4j_status;
```

Inspect Neo4j graph:

```bash
docker exec -it tessa-neo4j cypher-shell -u neo4j -p password
```

Then:

```cypher
MATCH (c:Chunk)
RETURN c.symbol_type, count(*) AS count
ORDER BY count DESC;
```

## Known Notes

- Graph retrieval depends on Neo4j graph nodes being indexed and Postgres chunks being present.
- Graph hydration and context expansion use Postgres by chunk ID and therefore do not calculate relevance scores.
- Elasticsearch remains the keyword backend, but it is no longer used for graph hydration or prev/next context expansion.
- Pinecone Local expects the configured embedding dimension, currently `1536`.
- The query service requires `neo4j` and `asyncpg` Python dependencies for live graph/Postgres retrieval.
- `pytest` is listed as a dev dependency for query-service tests.

# answer-eval-service

Python runner for comparing `/answer` modes in `query-service`.

It calls:

```text
POST /answer
```

for a set of questions across these modes:

```text
baseline
reasoning
agentic
```

and writes:

```text
CSV metrics
JSONL raw responses
PNG plots
```

## Run with Docker Compose

From the repo root:

```bash
docker compose --profile eval run --rm answer-eval-service
```

Results are written to:

```text
eval-results/
```

Each run creates:

```text
answer-eval-<timestamp>.csv
answer-eval-<timestamp>.jsonl
plots-<timestamp>/
  latency_by_mode.png
  latency_by_question.png
  tokens_by_mode.png
  tokens_by_question.png
  retrieval_volume_by_mode.png
  source_mix_by_mode.png
```

## Configure a run

Use environment variables:

```bash
EVAL_REPEATS=3 \
EVAL_REPO_URL=https://github.com/example/repo.git \
EVAL_SNAPSHOT_ID=your-snapshot-id \
docker compose --profile eval run --rm answer-eval-service
```

Common settings:

| Variable | Default | Purpose |
|---|---|---|
| `QUERY_SERVICE_URL` | `http://query-service:8082` in Docker | Query service base URL |
| `EVAL_QUESTIONS_FILE` | `questions/cpp-small.json` | Question set |
| `EVAL_OUTPUT_DIR` | `/app/results` in Docker | Output directory |
| `EVAL_MODES` | `baseline,reasoning,agentic` | Modes to compare |
| `EVAL_REPEATS` | `1` | Runs per question/mode |
| `EVAL_REPO_URL` | unset | Optional repo scope |
| `EVAL_BRANCH` | `main` | Branch scope |
| `EVAL_SNAPSHOT_ID` | unset | Optional snapshot scope |
| `EVAL_TOP_K` | `8` | Retrieval limit |
| `EVAL_CONTEXT_TOKEN_BUDGET` | `12000` | Context budget |
| `EVAL_TIMEOUT_SECONDS` | `180` | HTTP timeout per request |
| `EVAL_PLOTS` | `true` | Generate matplotlib PNG plots |

## Metrics

The CSV includes:

```text
latency_ms
answer_chars
answer_words
input_tokens
output_tokens
total_tokens
limitations_count
citation_count
citation_unique_chunks
citation_duplicate_chunks
citation_unique_files
context_block_count
context_unique_chunks
context_duplicate_chunks
context_unique_files
context_total_chars
context_avg_chars
context_avg_score
context_max_score
source_keyword_count
source_vector_count
source_graph_count
source_context_expansion_count
source_other_count
```

The JSONL output keeps each raw response next to its request and metrics so you can manually score answer quality later.

Token metrics come from the `token_usage` object returned by `/answer`. For strategy runs, this is aggregate LLM usage for the whole answer request, including planning/query-understanding calls and the final answer call.

## Plots

When `EVAL_PLOTS=true`, the runner generates:

```text
latency_by_mode.png
latency_by_question.png
tokens_by_mode.png
tokens_by_question.png
retrieval_volume_by_mode.png
source_mix_by_mode.png
```

Disable plots with:

```bash
EVAL_PLOTS=false docker compose --profile eval run --rm answer-eval-service
```

## Run locally

First make sure `query-service` is already running and reachable from your host:

```bash
curl http://localhost:8082/healthz
```

Then install and run the eval service from this directory:

```bash
cd /Users/rohandave/Documents/Projects/tessa-rag/services/answer-eval-service
python -m venv .venv
. .venv/bin/activate
pip install .
```

Run the default questions in `questions/cpp-small.json` against the rideshare repo for all modes:

```bash
EVAL_REPO_URL=https://github.com/rohanddave/rideshare \
EVAL_CONTEXT_TOKEN_BUDGET=50000 \
EVAL_TOP_K=20 \
EVAL_MODES=baseline,reasoning,agentic \
python -m app.main --query-service-url http://localhost:8082
```

Local results are written to:

```text
services/answer-eval-service/results/
```

Each run creates:

```text
answer-eval-<timestamp>.csv
answer-eval-<timestamp>.jsonl
plots-<timestamp>/
```

If retrieval returns empty results, the repo may have been indexed with a `.git` suffix. Try:

```bash
EVAL_REPO_URL=https://github.com/rohanddave/rideshare.git \
EVAL_CONTEXT_TOKEN_BUDGET=50000 \
EVAL_TOP_K=20 \
EVAL_MODES=baseline,reasoning,agentic \
python -m app.main --query-service-url http://localhost:8082
```

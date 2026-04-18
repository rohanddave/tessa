# query-service

Python query-time service for Tessa RAG.

This service keeps the query path in one process while preserving internal service boundaries:

1. Answer strategy selection
2. Retrieval Orchestrator
3. Context Assembly Service
4. LLM Answering Service

## Local endpoints

- `GET /healthz`
- `POST /retrieve`
- `POST /answer`

## Answer strategies

`POST /answer` supports three modes.

### Baseline

Baseline is the direct path:

```text
original query -> retrieve -> answer
```

It does not run query understanding. The original query is passed directly to retrieval, then the retrieved context is assembled and answered.

### Reasoning

Reasoning does one planning pass before retrieval:

```text
original query understanding
-> subquery generation
-> per-subquery retrieval planning
-> retrieve
-> fuse
-> answer
```

It understands the original query, generates retrieval-focused subqueries, plans retrieval for each subquery, retrieves chunks, fuses the results, and answers from the assembled context.

### Agentic

Agentic plans retrieval iteratively:

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

It understands the original query, generates one next retrieval step at a time, retrieves with that step's plan, observes the returned hits, and repeats until enough context has been found or the step limit is reached.

Use the `mode` field on `/answer`:

```json
{
  "query": "where is InvoiceService used?",
  "mode": "agentic"
}
```

Supported values are `baseline`, `reasoning`, and `agentic`. If omitted, `mode` defaults to `baseline`.

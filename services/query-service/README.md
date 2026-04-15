# query-service

Python query-time service for Tessa RAG.

This service keeps the query path in one process while preserving internal service boundaries:

1. Retrieval Orchestrator
2. Context Assembly Service
3. LLM Answering Service

## Local endpoints

- `GET /healthz`
- `POST /retrieve`
- `POST /answer`

The current implementation is a bootable scaffold. Retriever adapters return empty results until the Elasticsearch, Pinecone, Neo4j, and Postgres query implementations are filled in.

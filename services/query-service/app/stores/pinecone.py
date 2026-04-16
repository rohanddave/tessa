from typing import Any

import httpx

from app.config import Settings
from app.models.chunk import retrieval_hit_from_chunk
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit


class PineconeStore:
    def __init__(self, settings: Settings) -> None:
        self.host = settings.pinecone_host
        self.api_key = settings.pinecone_api_key
        self.api_version = settings.pinecone_api_version
        self.index = settings.pinecone_index
        self.namespace = settings.pinecone_namespace

    async def query(self, request: QueryRequest, vector: list[float], *, top_k: int) -> list[RetrievalHit]:
        if not vector:
            return []

        body: dict[str, Any] = {
            "namespace": self.namespace or "__default__",
            "vector": vector,
            "topK": top_k,
            "includeMetadata": True,
        }
        filters = self._filters(request)
        if filters:
            body["filter"] = filters

        async with httpx.AsyncClient(timeout=10) as client:
            response = await client.post(
                f"{self.host.rstrip('/')}/query",
                json=body,
                headers=self._headers(),
            )
            response.raise_for_status()
            data = response.json()

        results: list[RetrievalHit] = []
        for match in data.get("matches", []):
            metadata = match.get("metadata") or {}
            if "chunk_id" not in metadata and match.get("id"):
                metadata["chunk_id"] = match["id"]
            hit = retrieval_hit_from_chunk(metadata, score=float(match.get("score") or 0), source="vector")
            if hit is not None:
                results.append(hit)

        return results

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self.api_key:
            headers["Api-Key"] = self.api_key
        if self.api_version:
            headers["X-Pinecone-Api-Version"] = self.api_version
        return headers

    def _filters(self, request: QueryRequest) -> dict[str, Any]:
        filters: dict[str, Any] = {}
        if request.repo_url:
            filters["repo_url"] = {"$eq": request.repo_url}
        if request.branch:
            filters["branch"] = {"$eq": request.branch}
        if request.snapshot_id:
            filters["snapshot_id"] = {"$eq": request.snapshot_id}
        return filters

from __future__ import annotations

from typing import Any
from urllib.parse import quote

import httpx

from app.config import Settings
from app.models.chunk import retrieval_hit_from_chunk
from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit


class ElasticsearchStore:
    def __init__(self, settings: Settings) -> None:
        self.url = settings.elasticsearch_url
        self.index = settings.elasticsearch_index

    async def search(
        self,
        request: QueryRequest,
        understanding: QueryUnderstanding,
        *,
        top_k: int,
    ) -> list[RetrievalHit]:
        body = {
            "size": top_k,
            "query": {
                "bool": {
                    "must": [
                        {
                            "multi_match": {
                                "query": self._query_text(request, understanding),
                                "fields": [
                                    "content^4",
                                    "symbol_name^3",
                                    "file_path^3",
                                    "symbol_type^2",
                                    "language",
                                ],
                                "type": "best_fields",
                                "operator": "or",
                            }
                        }
                    ],
                    "filter": self._filters(request),
                }
            },
        }

        data = await self._post(f"/{quote(self.index)}/_search", body)
        hits = data.get("hits", {}).get("hits", [])

        results: list[RetrievalHit] = []
        for hit in hits:
            source = hit.get("_source") or {}
            result = retrieval_hit_from_chunk(source, score=float(hit.get("_score") or 0), source="keyword")
            if result is not None:
                results.append(result)

        return results

    async def mget(self, chunk_ids: list[str], *, source: str = "context_expansion") -> list[RetrievalHit]:
        ids = [chunk_id for chunk_id in dict.fromkeys(chunk_ids) if chunk_id]
        if not ids:
            return []

        data = await self._post(f"/{quote(self.index)}/_mget", {"ids": ids})
        docs = data.get("docs", [])

        results: list[RetrievalHit] = []
        for doc in docs:
            if not doc.get("found"):
                continue
            hit = retrieval_hit_from_chunk(doc.get("_source") or {}, score=0, source=source)
            if hit is not None:
                results.append(hit)

        return results

    async def _post(self, path: str, body: dict[str, Any]) -> dict[str, Any]:
        async with httpx.AsyncClient(timeout=10) as client:
            response = await client.post(f"{self.url.rstrip('/')}{path}", json=body)
            response.raise_for_status()
            return response.json()

    def _query_text(self, request: QueryRequest, understanding: QueryUnderstanding) -> str:
        parts = [request.query, *understanding.entities]
        return " ".join(part for part in parts if part)

    def _filters(self, request: QueryRequest) -> list[dict[str, Any]]:
        filters: list[dict[str, Any]] = []
        if request.repo_url:
            filters.append({"term": {"repo_url.keyword": request.repo_url}})
        if request.branch:
            filters.append({"term": {"branch.keyword": request.branch}})
        if request.snapshot_id:
            filters.append({"term": {"snapshot_id.keyword": request.snapshot_id}})
        return filters

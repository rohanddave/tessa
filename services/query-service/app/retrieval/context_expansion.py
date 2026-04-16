from __future__ import annotations

from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit
from app.stores.elasticsearch import ElasticsearchStore


class ContextExpansionService:
    def __init__(self, elasticsearch: ElasticsearchStore) -> None:
        self.elasticsearch = elasticsearch

    async def expand(self, request: QueryRequest, hits: list[RetrievalHit]) -> list[RetrievalHit]:
        neighbor_ids = []
        existing_ids = {hit.chunk_id for hit in hits}
        for hit in hits:
            for chunk_id in [hit.prev_chunk_id, hit.next_chunk_id]:
                if chunk_id and chunk_id not in existing_ids:
                    neighbor_ids.append(chunk_id)

        neighbors = await self.elasticsearch.mget(neighbor_ids)
        allowed = self._filter_to_request_scope(request, neighbors)
        return self._merge_preserving_primary_order(hits, allowed)

    def _filter_to_request_scope(self, request: QueryRequest, hits: list[RetrievalHit]) -> list[RetrievalHit]:
        scoped: list[RetrievalHit] = []
        for hit in hits:
            if request.repo_url and hit.repo_url != request.repo_url:
                continue
            if request.branch and hit.branch != request.branch:
                continue
            if request.snapshot_id and hit.snapshot_id != request.snapshot_id:
                continue
            scoped.append(hit)
        return scoped

    def _merge_preserving_primary_order(
        self,
        primary: list[RetrievalHit],
        neighbors: list[RetrievalHit],
    ) -> list[RetrievalHit]:
        by_id = {neighbor.chunk_id: neighbor for neighbor in neighbors}
        emitted: set[str] = set()
        merged: list[RetrievalHit] = []

        for hit in primary:
            for chunk_id in [hit.prev_chunk_id, hit.chunk_id, hit.next_chunk_id]:
                if not chunk_id or chunk_id in emitted:
                    continue
                candidate = hit if chunk_id == hit.chunk_id else by_id.get(chunk_id)
                if candidate is None:
                    continue
                emitted.add(candidate.chunk_id)
                merged.append(candidate)

        return merged

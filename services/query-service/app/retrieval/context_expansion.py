from __future__ import annotations

import logging

from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit
from app.stores.postgres import PostgresStore


class ContextExpansionService:
    def __init__(self, postgres: PostgresStore, logger: logging.Logger | None = None) -> None:
        self.postgres = postgres
        self.logger = logger or logging.getLogger(__name__)

    async def expand(self, request: QueryRequest, hits: list[RetrievalHit]) -> list[RetrievalHit]:
        neighbor_ids = []
        existing_ids = {hit.chunk_id for hit in hits}
        for hit in hits:
            for chunk_id in [hit.prev_chunk_id, hit.next_chunk_id]:
                if chunk_id and chunk_id not in existing_ids:
                    neighbor_ids.append(chunk_id)

        self.logger.info(
            "context expansion fetching neighbors base_hits=%d neighbor_ids=%d",
            len(hits),
            len(neighbor_ids),
        )
        neighbors = await self.postgres.mget_chunks(neighbor_ids, source="context_expansion")
        allowed = self._filter_to_request_scope(request, neighbors)
        merged = self._merge_preserving_primary_order(hits, allowed)
        self.logger.info(
            "context expansion merged fetched_neighbors=%d scoped_neighbors=%d merged_hits=%d",
            len(neighbors),
            len(allowed),
            len(merged),
        )
        return merged

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

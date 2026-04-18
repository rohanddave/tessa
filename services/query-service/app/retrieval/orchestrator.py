from __future__ import annotations

import asyncio
import logging
import time
from collections import Counter

from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit, RetrievalResult
from app.retrieval.context_expansion import ContextExpansionService
from app.retrieval.fusion import FusionRanker
from app.retrieval.query_understanding import QueryUnderstandingService
from app.retrieval.retrievers import Retriever


class RetrievalOrchestrator:
    chunks: list[list[RetrievalHit]]
    fused_chunks: list[RetrievalHit]
    expanded_chunks: list[RetrievalHit]

    def __init__(
        self,
        logger: logging.Logger,
        query_understanding: QueryUnderstandingService, 
        retrievers: list[Retriever] | None = None,
        fusion_ranker: FusionRanker | None = None,
        context_expansion: ContextExpansionService | None = None,
    ) -> None:
        self.logger = logger
        self.query_understanding = query_understanding
        self.retrievers = retrievers or []
        self.fusion_ranker = fusion_ranker or FusionRanker()
        if context_expansion is None:
            raise ValueError("context_expansion is required")
        self.context_expansion = context_expansion
        self.chunks = []
        self.fused_chunks = []
        self.expanded_chunks = []

    async def retrieve(self, request: QueryRequest) -> RetrievalResult:
        started = time.monotonic()
        understanding = await self.query_understanding.understand(request.query)

        self.chunks = []
        self.fused_chunks = []
        self.expanded_chunks = []
        await self.retreive(request, understanding)
        self.fuse(request.top_k)
        await self.expand(request)

        self.logger.info(
            "retrieval completed final_hits=%d duration_ms=%.2f",
            len(self.expanded_chunks),
            (time.monotonic() - started) * 1000,
        )
        return RetrievalResult(understanding=understanding, hits=self.expanded_chunks)

    async def retreive(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[list[RetrievalHit]]:
        selected = [r for r in self.retrievers if r.name in understanding.retrieval_plan]

        self.logger.info(
            "retrieval started retrievers=%s intent=%s entities=%d top_k=%d repo_scoped=%s snapshot_scoped=%s",
            [r.name for r in selected],
            understanding.intent,
            len(understanding.entities),
            request.top_k,
            bool(request.repo_url),
            bool(request.snapshot_id),
        )
        raw_results = await asyncio.gather(
            *(retriever.retrieve(request, understanding) for retriever in selected),
            return_exceptions=True,
        )

        result_sets = []
        for retriever, result in zip(selected, raw_results):
            if isinstance(result, Exception):
                self.logger.warning("retriever failed name=%s error=%s", retriever.name, result)
                continue
            self.logger.info(
                "retriever completed name=%s hits=%d source_counts=%s",
                retriever.name,
                len(result),
                dict(Counter(source for hit in result for source in hit.sources)),
            )
            self.chunks.append(result)
            result_sets.append(result)

        return result_sets

    def fuse(self, limit: int) -> list[RetrievalHit]:
        fusion_started = time.monotonic()
        self.fused_chunks = self.fusion_ranker.fuse(self.chunks, limit)
        self.logger.info(
            "fusion completed input_sets=%d raw_counts=%s fused_hits=%d duration_ms=%.2f",
            len(self.chunks),
            {f"set_{idx}": len(hits) for idx, hits in enumerate(self.chunks, start=1)},
            len(self.fused_chunks),
            (time.monotonic() - fusion_started) * 1000,
        )
        return self.fused_chunks

    async def expand(self, request: QueryRequest) -> list[RetrievalHit]:
        try:
            expansion_started = time.monotonic()
            self.expanded_chunks = await self.context_expansion.expand(request, self.fused_chunks)
            self.logger.info(
                "context expansion completed base_hits=%d expanded_hits=%d duration_ms=%.2f",
                len(self.fused_chunks),
                len(self.expanded_chunks),
                (time.monotonic() - expansion_started) * 1000,
            )
        except Exception as err:
            self.logger.warning("context expansion failed error=%s", err)
            self.expanded_chunks = self.fused_chunks
        return self.expanded_chunks

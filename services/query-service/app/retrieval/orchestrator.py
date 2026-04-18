from __future__ import annotations

import asyncio
import logging
import time
from collections import Counter

from app.models.query import QueryRequest
from app.models.retrieval import RetrievalResult
from app.retrieval.context_expansion import ContextExpansionService
from app.retrieval.fusion import FusionRanker
from app.retrieval.query_understanding import QueryUnderstandingService
from app.retrieval.retrievers import Retriever


class RetrievalOrchestrator:
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

    async def retrieve(self, request: QueryRequest) -> RetrievalResult:
        started = time.monotonic()
        understanding = await self.query_understanding.understand(request.query)
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
        raw_counts: dict[str, int] = {}
        for retriever, result in zip(selected, raw_results):
            if isinstance(result, Exception):
                self.logger.warning("retriever failed name=%s error=%s", retriever.name, result)
                continue
            raw_counts[retriever.name] = len(result)
            self.logger.info(
                "retriever completed name=%s hits=%d source_counts=%s",
                retriever.name,
                len(result),
                dict(Counter(source for hit in result for source in hit.sources)),
            )
            result_sets.append(result)

        fusion_started = time.monotonic()
        fused = self.fusion_ranker.fuse(result_sets, request.top_k)
        self.logger.info(
            "fusion completed input_sets=%d raw_counts=%s fused_hits=%d duration_ms=%.2f",
            len(result_sets),
            raw_counts,
            len(fused),
            (time.monotonic() - fusion_started) * 1000,
        )
        try:
            expansion_started = time.monotonic()
            expanded = await self.context_expansion.expand(request, fused)
            self.logger.info(
                "context expansion completed base_hits=%d expanded_hits=%d duration_ms=%.2f",
                len(fused),
                len(expanded),
                (time.monotonic() - expansion_started) * 1000,
            )
        except Exception as err:
            self.logger.warning("context expansion failed error=%s", err)
            expanded = fused
        self.logger.info(
            "retrieval completed final_hits=%d duration_ms=%.2f",
            len(expanded),
            (time.monotonic() - started) * 1000,
        )
        return RetrievalResult(understanding=understanding, hits=expanded)

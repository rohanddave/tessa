from __future__ import annotations

import asyncio
import logging

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
        query_understanding: QueryUnderstandingService | None = None,
        retrievers: list[Retriever] | None = None,
        fusion_ranker: FusionRanker | None = None,
        context_expansion: ContextExpansionService | None = None,
    ) -> None:
        self.logger = logger
        self.query_understanding = query_understanding or QueryUnderstandingService()
        self.retrievers = retrievers or []
        self.fusion_ranker = fusion_ranker or FusionRanker()
        if context_expansion is None:
            raise ValueError("context_expansion is required")
        self.context_expansion = context_expansion

    async def retrieve(self, request: QueryRequest) -> RetrievalResult:
        understanding = self.query_understanding.understand(request.query)
        selected = [r for r in self.retrievers if r.name in understanding.retrieval_plan]

        self.logger.info("retrieving query with retrievers=%s", [r.name for r in selected])
        raw_results = await asyncio.gather(
            *(retriever.retrieve(request, understanding) for retriever in selected),
            return_exceptions=True,
        )

        result_sets = []
        for retriever, result in zip(selected, raw_results):
            if isinstance(result, Exception):
                self.logger.warning("retriever failed name=%s error=%s", retriever.name, result)
                continue
            result_sets.append(result)

        fused = self.fusion_ranker.fuse(result_sets, request.top_k)
        try:
            expanded = await self.context_expansion.expand(request, fused)
        except Exception as err:
            self.logger.warning("context expansion failed error=%s", err)
            expanded = fused
        return RetrievalResult(understanding=understanding, hits=expanded)

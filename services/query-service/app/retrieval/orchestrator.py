import asyncio
import logging

from app.models.query import QueryRequest
from app.models.retrieval import RetrievalResult
from app.retrieval.context_expansion import ContextExpansionService
from app.retrieval.fusion import FusionRanker
from app.retrieval.query_understanding import QueryUnderstandingService
from app.retrieval.retrievers import GraphRetriever, KeywordRetriever, Retriever, VectorRetriever


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
        self.retrievers = retrievers or [KeywordRetriever(), VectorRetriever(), GraphRetriever()]
        self.fusion_ranker = fusion_ranker or FusionRanker()
        self.context_expansion = context_expansion or ContextExpansionService()

    async def retrieve(self, request: QueryRequest) -> RetrievalResult:
        understanding = self.query_understanding.understand(request.query)
        selected = [r for r in self.retrievers if r.name in understanding.retrieval_plan]

        self.logger.info("retrieving query with retrievers=%s", [r.name for r in selected])
        result_sets = await asyncio.gather(
            *(retriever.retrieve(request, understanding) for retriever in selected)
        )

        fused = self.fusion_ranker.fuse(result_sets, request.top_k)
        expanded = await self.context_expansion.expand(request, fused)
        return RetrievalResult(understanding=understanding, hits=expanded[: request.top_k])

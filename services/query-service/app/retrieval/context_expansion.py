from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit


class ContextExpansionService:
    async def expand(self, request: QueryRequest, hits: list[RetrievalHit]) -> list[RetrievalHit]:
        # Placeholder for prev/next chunk, file-neighbor, and graph-neighbor expansion.
        # Keep this as its own service so expansion can become store-backed without
        # changing the retrieval orchestrator contract.
        return hits

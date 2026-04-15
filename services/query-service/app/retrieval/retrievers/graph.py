from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit


class GraphRetriever:
    name = "graph"

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        # TODO: Query Neo4j for symbol, import, and relationship neighbors.
        return []

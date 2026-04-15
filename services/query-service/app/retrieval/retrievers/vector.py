from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit


class VectorRetriever:
    name = "vector"

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        # TODO: Embed query and query Pinecone local index.
        return []

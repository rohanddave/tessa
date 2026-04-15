from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit


class KeywordRetriever:
    name = "keyword"

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        # TODO: Query Elasticsearch BM25 against configured chunk index.
        return []

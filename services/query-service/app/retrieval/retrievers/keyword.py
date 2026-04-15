from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit
from app.stores.elasticsearch import ElasticsearchStore


class KeywordRetriever:
    name = "keyword"

    def __init__(self, elasticsearch: ElasticsearchStore) -> None:
        self.elasticsearch = elasticsearch

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        return await self.elasticsearch.search(request, understanding, top_k=request.top_k)

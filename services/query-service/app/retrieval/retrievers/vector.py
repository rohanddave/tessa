from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit
from app.stores.openai import OpenAIEmbeddingStore
from app.stores.pinecone import PineconeStore


class VectorRetriever:
    name = "vector"

    def __init__(self, embeddings: OpenAIEmbeddingStore, pinecone: PineconeStore) -> None:
        self.embeddings = embeddings
        self.pinecone = pinecone

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        vector = await self.embeddings.embed(request.query)
        if not vector:
            return []
        return await self.pinecone.query(request, vector, top_k=request.top_k)

from typing import Protocol

from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit, QueryUnderstanding


class Retriever(Protocol):
    name: str

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        ...

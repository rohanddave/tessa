import logging

from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit
from app.stores.neo4j import Neo4jStore
from app.stores.postgres import PostgresStore


class GraphRetriever:
    name = "graph"

    def __init__(
        self,
        neo4j: Neo4jStore,
        postgres: PostgresStore,
        logger: logging.Logger | None = None,
    ) -> None:
        self.neo4j = neo4j
        self.postgres = postgres
        self.logger = logger or logging.getLogger(__name__)

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        graph_hits = await self.neo4j.search(request, understanding.entities, top_k=request.top_k * 3)
        if not graph_hits:
            self.logger.info("graph retriever completed graph_hits=0 hydrated_hits=0 returned_hits=0")
            return []

        by_id = {hit.chunk_id: hit for hit in graph_hits}
        hydrated = await self.postgres.mget_chunks(list(by_id), source="graph")
        hydrated_by_id = {hit.chunk_id: hit for hit in hydrated}

        results: list[RetrievalHit] = []
        for graph_hit in graph_hits:
            hit = hydrated_by_id.get(graph_hit.chunk_id) or graph_hit
            hit.score = max(hit.score, graph_hit.score)
            hit.sources = sorted(set(hit.sources + graph_hit.sources))
            results.append(hit)
            if len(results) >= request.top_k:
                break

        self.logger.info(
            "graph retriever completed graph_hits=%d hydrated_hits=%d returned_hits=%d",
            len(graph_hits),
            len(hydrated),
            len(results),
        )
        return results

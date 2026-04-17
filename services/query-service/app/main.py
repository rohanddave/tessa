from __future__ import annotations

import logging

from fastapi import Depends, FastAPI

from app.answering import LLMAnsweringService
from app.answering.llm_client import LLMClient
from app.config import Settings, get_settings
from app.context import ContextAssemblyService
from app.models.answer import AnswerRequest, AnswerResponse
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalResponse
from app.retrieval.context_expansion import ContextExpansionService
from app.retrieval.retrievers import GraphRetriever, KeywordRetriever, VectorRetriever
from app.retrieval import RetrievalOrchestrator
from app.stores.elasticsearch import ElasticsearchStore
from app.stores.neo4j import Neo4jStore
from app.stores.openai import OpenAIEmbeddingStore
from app.stores.pinecone import PineconeStore
from app.stores.postgres import PostgresStore


def build_app() -> FastAPI:
    app = FastAPI(title="Tessa Query Service", version="0.1.0")

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.post("/retrieve", response_model=RetrievalResponse)
    async def retrieve(
        request: QueryRequest,
        services: QueryServices = Depends(get_query_services),
    ) -> RetrievalResponse:
        result = await services.retrieval.retrieve(request)
        return RetrievalResponse(query=request.query, result=result)

    @app.post("/answer", response_model=AnswerResponse)
    async def answer(
        request: AnswerRequest,
        services: QueryServices = Depends(get_query_services),
    ) -> AnswerResponse:
        retrieval_result = await services.retrieval.retrieve(request)
        assembled_context = await services.context_assembly.assemble(request, retrieval_result)
        return await services.answering.answer(assembled_context)

    return app


class QueryServices:
    def __init__(self, settings: Settings) -> None:
        logger = logging.getLogger(settings.service_name)
        elasticsearch = ElasticsearchStore(settings)
        neo4j = Neo4jStore(settings)
        postgres = PostgresStore(settings)
        pinecone = PineconeStore(settings)
        embeddings = OpenAIEmbeddingStore(settings)

        self.retrieval = RetrievalOrchestrator(
            logger=logger,
            retrievers=[
                KeywordRetriever(elasticsearch),
                VectorRetriever(embeddings, pinecone),
                GraphRetriever(neo4j, postgres, logger),
            ],
            context_expansion=ContextExpansionService(postgres, logger),
        )
        self.context_assembly = ContextAssemblyService(logger)
        self.answering = LLMAnsweringService(LLMClient(settings))


def get_query_services(settings: Settings = Depends(get_settings)) -> QueryServices:
    return QueryServices(settings)


app = build_app()

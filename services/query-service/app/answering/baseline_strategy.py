from app.models.answer import AnswerRequest, AnswerResponse
from app.answering.answering_strategy import AnsweringStrategy
from app.stores.openai import OpenAIEmbeddingStore
from app.stores.pinecone import PineconeStore
from app.stores.postgres import PostgresStore
import logging
from app.stores.elasticsearch import ElasticsearchStore
from app.stores.neo4j import Neo4jStore
from app.config import Settings
from app.retrieval import RetrievalOrchestrator
from app.retrieval.retrievers import GraphRetriever, KeywordRetriever, VectorRetriever
from app.retrieval.context_expansion import ContextExpansionService
from app.context import ContextAssemblyService
from app.answering.llm_client import LLMClient
from app.answering import LLMAnsweringService
from app.retrieval.query_understanding import SystemQueryUnderstandingService

class BaselineRAGStrategy(AnsweringStrategy): 
    def __init__(self, settings: Settings) -> None: 
        logger = logging.getLogger(settings.service_name)
        elasticsearch = ElasticsearchStore(settings)
        neo4j = Neo4jStore(settings)
        postgres = PostgresStore(settings)
        pinecone = PineconeStore(settings)
        embeddings = OpenAIEmbeddingStore(settings)

        self.retrieval = RetrievalOrchestrator(
            logger=logger,
            query_understanding=SystemQueryUnderstandingService(),
            retrievers=[
                KeywordRetriever(elasticsearch),
                VectorRetriever(embeddings, pinecone),
                GraphRetriever(neo4j, postgres, logger),
            ],
            context_expansion=ContextExpansionService(postgres, logger),
        )
        self.context_assembly = ContextAssemblyService(logger)
        self.llm_client = LLMClient(settings)
        self.answering_service = LLMAnsweringService(self.llm_client)
      
    async def answer(self, request: AnswerRequest) -> AnswerResponse:
        self.llm_client.reset_token_usage()
        retrieval_result = await self.retrieval.retrieve(request)
        assembled_context = await self.context_assembly.assemble(request, retrieval_result)
        response = await self.answering_service.answer(assembled_context)
        response.token_usage = self.llm_client.token_usage_snapshot()
        return response

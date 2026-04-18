from app.models.answer import AnswerRequest
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
from app.retrieval.query_understanding import LLMQueryUnderstandingService
from app.models.retrieval import QueryUnderstanding
import json
from typing import Any

class ReasoningRAGStrategy(AnsweringStrategy): 
    def __init__(self, settings: Settings) -> None: 
        logger = logging.getLogger(settings.service_name)
        elasticsearch = ElasticsearchStore(settings)
        neo4j = Neo4jStore(settings)
        postgres = PostgresStore(settings)
        pinecone = PineconeStore(settings)
        embeddings = OpenAIEmbeddingStore(settings)

        self.llm_client = LLMClient(settings) 
        self.query_understanding = LLMQueryUnderstandingService(llm_client=self.llm_client)
        self.retrieval = RetrievalOrchestrator(
            logger=logger,
            query_understanding= self.query_understanding,
            retrievers=[
                KeywordRetriever(elasticsearch),
                VectorRetriever(embeddings, pinecone),
                GraphRetriever(neo4j, postgres, logger),
            ],
            context_expansion=ContextExpansionService(postgres, logger),
        )
        self.context_assembly = ContextAssemblyService(logger)
        self.answering_service = LLMAnsweringService(self.llm_client)
      
    async def answer(self, request: AnswerRequest): 
        initial_query_understanding = self.query_understanding.understand(request.query)
        subqueries = await self._generate_subqueries(request.query, initial_query_understanding)

    
    async def _generate_subqueries(
        self,
        query: str,
        query_understanding: QueryUnderstanding,
    ) -> list[str]:
        intent = query_understanding.intent
        entities = query_understanding.entities

        prompt = """
    You are a subquery generation engine for a codebase retrieval system.

    Your task is to break a user's query into a small set of effective search queries that can be used to retrieve relevant code.

    You are NOT answering the question.
    You are ONLY generating search queries.

    Return ONLY valid JSON in this format:

    {
    "subqueries": ["string"]
    }

    Guidelines:

    - Generate 2 to 6 subqueries
    - Include at least one query that preserves exact user terms (identifiers, filenames, symbols)
    - Include semantic variations to improve recall
    - Keep queries short and retrieval-focused
    - Avoid redundancy

    Use intent to guide decomposition:
    - code_search → definitions, locations
    - debugging → failure conditions, edge cases
    - execution_flow → sequence, pipeline, request flow
    - dependency_analysis → imports, callers, relationships
    - code_explanation → behavior and functionality

    Use entities if present:
    - preserve them in at least one query

    Do NOT:
    - answer the question
    - include explanations

    ---

    Query:
    {query}

    Intent:
    {intent}

    Entities:
    {entities}

    Return JSON only.
    """.strip().format(
            query=query,
            intent=intent,
            entities=json.dumps(entities),
        )

        try:
            raw_response = await self.llm_client.complete(prompt=prompt)
            content = self._extract_json(raw_response)
            data = json.loads(content)

            subqueries = data.get("subqueries", [])

            # validate
            validated: list[str] = []
            for sq in subqueries:
                if isinstance(sq, str):
                    sq = sq.strip()
                    if sq:
                        validated.append(sq)

            # ensure at least one entity-preserving query
            if entities:
                has_entity_query = any(
                    any(entity.lower() in sq.lower() for entity in entities)
                    for sq in validated
                )
                if not has_entity_query:
                    validated.insert(0, " ".join(entities))

            # fallback if empty
            if not validated:
                return self._fallback_subqueries(query, entities)

            # dedupe
            seen = set()
            deduped = []
            for sq in validated:
                key = sq.lower()
                if key in seen:
                    continue
                seen.add(key)
                deduped.append(sq)

            return deduped[:6]

        except Exception:
            return self._fallback_subqueries(query, entities)


    def _extract_json(self, text: str) -> str:
        text = text.strip()

        if text.startswith("```"):
            lines = text.splitlines()
            if len(lines) >= 3:
                return "\n".join(lines[1:-1]).strip()

        return text


    def _fallback_subqueries(
        self,
        query: str,
        entities: list[str],
    ) -> list[str]:
        subqueries = []

        if entities:
            subqueries.append(" ".join(entities))

        subqueries.append(query)

        return subqueries[:6]

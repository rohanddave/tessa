from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field

from app.answering import LLMAnsweringService
from app.answering.answering_strategy import AnsweringStrategy
from app.answering.llm_client import LLMClient
from app.config import Settings
from app.context import ContextAssemblyService
from app.models.answer import AnswerRequest, AnswerResponse
from app.models.retrieval import QueryUnderstanding, RetrievalHit, RetrievalResult
from app.retrieval import RetrievalOrchestrator
from app.retrieval.context_expansion import ContextExpansionService
from app.retrieval.query_understanding import LLMQueryUnderstandingService
from app.retrieval.retrievers import GraphRetriever, KeywordRetriever, VectorRetriever
from app.stores.elasticsearch import ElasticsearchStore
from app.stores.neo4j import Neo4jStore
from app.stores.openai import OpenAIEmbeddingStore
from app.stores.pinecone import PineconeStore
from app.stores.postgres import PostgresStore


VALID_RETRIEVERS = {"keyword", "vector", "graph"}


@dataclass
class AgenticRetrievalStep:
    subquery: str
    intent: str
    retrieval_plan: list[str]
    entities: list[str] = field(default_factory=list)
    done: bool = False
    rationale: str = ""


class AgenticRAGStrategy(AnsweringStrategy):
    max_steps = 4

    def __init__(self, settings: Settings) -> None:
        logger = logging.getLogger(settings.service_name)
        elasticsearch = ElasticsearchStore(settings)
        neo4j = Neo4jStore(settings)
        postgres = PostgresStore(settings)
        pinecone = PineconeStore(settings)
        embeddings = OpenAIEmbeddingStore(settings)

        self.logger = logger
        self.llm_client = LLMClient(settings)
        self.query_understanding = LLMQueryUnderstandingService(llm_client=self.llm_client)
        self.retrieval = RetrievalOrchestrator(
            logger=logger,
            query_understanding=self.query_understanding,
            retrievers=[
                KeywordRetriever(elasticsearch),
                VectorRetriever(embeddings, pinecone),
                GraphRetriever(neo4j, postgres, logger),
            ],
            context_expansion=ContextExpansionService(postgres, logger),
        )
        self.context_assembly = ContextAssemblyService(logger)
        self.answering_service = LLMAnsweringService(self.llm_client)

    async def answer(self, request: AnswerRequest) -> AnswerResponse:
        self.llm_client.reset_token_usage()
        original_understanding = await self.query_understanding.understand(request.query)
        observations: list[str] = []
        used_subqueries: set[str] = set()

        self.retrieval.chunks = []
        self.retrieval.fused_chunks = []
        self.retrieval.expanded_chunks = []

        for step_number in range(1, self.max_steps + 1):
            step = await self._generate_next_step(
                request.query,
                original_understanding,
                observations,
                used_subqueries,
                step_number,
            )
            if step.done:
                self.logger.info("agentic retrieval stopped step=%d rationale=%s", step_number, step.rationale)
                break

            used_subqueries.add(step.subquery.lower())
            understanding = QueryUnderstanding(
                intent=step.intent,
                entities=step.entities,
                retrieval_plan=step.retrieval_plan,
            )
            subquery_request = request.model_copy(update={"query": step.subquery})
            result_sets = await self.retrieval.retreive(subquery_request, understanding)
            observations.append(self._observe_step(step_number, step, result_sets))

        if not self.retrieval.chunks:
            await self.retrieval.retreive(request, original_understanding)
            observations.append("Fallback step retrieved with the original query because no agentic step ran.")

        self.retrieval.fuse(request.top_k)
        expanded_hits = await self.retrieval.expand(request)
        retrieval_result = RetrievalResult(
            understanding=original_understanding,
            hits=expanded_hits,
        )
        assembled_context = await self.context_assembly.assemble(request, retrieval_result)
        response = await self.answering_service.answer(assembled_context)
        response.token_usage = self.llm_client.token_usage_snapshot()
        return response

    async def _generate_next_step(
        self,
        original_query: str,
        original_understanding: QueryUnderstanding,
        observations: list[str],
        used_subqueries: set[str],
        step_number: int,
    ) -> AgenticRetrievalStep:
        prompt = self._build_next_step_prompt(
            original_query,
            original_understanding,
            observations,
            used_subqueries,
            step_number,
        )

        try:
            raw_response = await self.llm_client.complete(prompt=prompt)
            data = json.loads(self._extract_json(raw_response))
            if not isinstance(data, dict):
                raise ValueError("agentic step response must be a JSON object")
            return self._parse_step(data, original_query, original_understanding, used_subqueries)
        except Exception as err:
            self.logger.warning("agentic next-step generation failed step=%d error=%s", step_number, err)
            return self._fallback_step(original_query, original_understanding, used_subqueries)

    def _build_next_step_prompt(
        self,
        original_query: str,
        original_understanding: QueryUnderstanding,
        observations: list[str],
        used_subqueries: set[str],
        step_number: int,
    ) -> str:
        prompt = """
You are an agentic retrieval planner for a codebase RAG system.

Use this loop:
1. Look at the original query understanding.
2. Review observations from prior retrieval steps.
3. Decide the single best next retrieval subquery and retrieval plan.
4. Stop only when the observations look sufficient or no useful next query remains.

Return ONLY valid JSON in exactly this format:

{
  "done": false,
  "subquery": "string",
  "intent": "code_search | debugging | code_explanation | execution_flow | dependency_analysis | configuration | general_code_question",
  "entities": ["string"],
  "retrieval_plan": ["keyword", "vector", "graph"],
  "rationale": "short reason"
}

Rules:
- Generate one next step, not a list.
- Use only these retrieval strategies: keyword, vector, graph.
- Use keyword for exact names, paths, symbols, and error text.
- Use vector for concepts, behavior, and fuzzy matching.
- Use graph for callers, imports, dependencies, flow, and relationships.
- Do not repeat prior subqueries.
- Set done to true only when enough relevant context has already been observed.
- If done is true, leave subquery empty and retrieval_plan empty.

Original query:
__QUERY__

Original understanding:
__UNDERSTANDING__

Prior subqueries:
__USED_SUBQUERIES__

Observations:
__OBSERVATIONS__

Step number:
__STEP_NUMBER__

Return JSON only.
""".strip()
        return (
            prompt.replace("__QUERY__", original_query)
            .replace("__UNDERSTANDING__", original_understanding.model_dump_json())
            .replace("__USED_SUBQUERIES__", json.dumps(sorted(used_subqueries)))
            .replace("__OBSERVATIONS__", json.dumps(observations[-6:]))
            .replace("__STEP_NUMBER__", str(step_number))
        )

    def _parse_step(
        self,
        data: dict,
        original_query: str,
        original_understanding: QueryUnderstanding,
        used_subqueries: set[str],
    ) -> AgenticRetrievalStep:
        done = bool(data.get("done", False))
        if done:
            return AgenticRetrievalStep(
                subquery="",
                intent=original_understanding.intent,
                retrieval_plan=[],
                done=True,
                rationale=str(data.get("rationale", "")),
            )

        subquery = str(data.get("subquery") or "").strip()
        if not subquery or subquery.lower() in used_subqueries:
            return self._fallback_step(original_query, original_understanding, used_subqueries)

        intent = str(data.get("intent") or original_understanding.intent)
        raw_entities = data.get("entities", [])
        if not isinstance(raw_entities, list):
            raw_entities = []
        entities = [item.strip() for item in raw_entities if isinstance(item, str) and item.strip()]
        retrieval_plan = self._valid_retrieval_plan(data.get("retrieval_plan", []))
        if not retrieval_plan:
            retrieval_plan = self._valid_retrieval_plan(original_understanding.retrieval_plan)

        return AgenticRetrievalStep(
            subquery=subquery,
            intent=intent,
            entities=entities,
            retrieval_plan=retrieval_plan,
            rationale=str(data.get("rationale", "")),
        )

    def _fallback_step(
        self,
        original_query: str,
        original_understanding: QueryUnderstanding,
        used_subqueries: set[str],
    ) -> AgenticRetrievalStep:
        candidates = []
        if original_understanding.entities:
            candidates.append(" ".join(original_understanding.entities))
        candidates.append(original_query)

        subquery = next((candidate for candidate in candidates if candidate.lower() not in used_subqueries), "")
        if not subquery:
            return AgenticRetrievalStep(
                subquery="",
                intent=original_understanding.intent,
                retrieval_plan=[],
                done=True,
                rationale="fallback subqueries were already used",
            )

        return AgenticRetrievalStep(
            subquery=subquery,
            intent=original_understanding.intent,
            entities=original_understanding.entities,
            retrieval_plan=self._valid_retrieval_plan(original_understanding.retrieval_plan),
            rationale="fallback retrieval step",
        )

    def _observe_step(
        self,
        step_number: int,
        step: AgenticRetrievalStep,
        result_sets: list[list[RetrievalHit]],
    ) -> str:
        hits = [hit for result_set in result_sets for hit in result_set]
        hit_summaries = []
        for hit in hits[:5]:
            hit_summaries.append(
                {
                    "chunk_id": hit.chunk_id,
                    "file_path": hit.file_path,
                    "symbol_name": hit.symbol_name,
                    "sources": hit.sources,
                    "score": hit.score,
                    "has_content": bool(hit.content),
                }
            )

        return json.dumps(
            {
                "step": step_number,
                "subquery": step.subquery,
                "retrieval_plan": step.retrieval_plan,
                "hit_count": len(hits),
                "hits": hit_summaries,
            }
        )

    def _valid_retrieval_plan(self, retrieval_plan: object) -> list[str]:
        if not isinstance(retrieval_plan, list):
            return ["keyword", "vector"]

        valid = []
        for retriever in retrieval_plan:
            if retriever in VALID_RETRIEVERS and retriever not in valid:
                valid.append(retriever)
        return valid or ["keyword", "vector"]

    def _extract_json(self, text: str) -> str:
        text = text.strip()

        if text.startswith("```"):
            lines = text.splitlines()
            if len(lines) >= 3:
                return "\n".join(lines[1:-1]).strip()

        return text

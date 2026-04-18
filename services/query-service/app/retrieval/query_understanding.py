import re
import json
from abc import ABC, abstractmethod

from app.models.retrieval import QueryUnderstanding
from app.answering.llm_client import LLMClient

class QueryUnderstandingService(ABC): 
    @abstractmethod
    async def understand(self, query: str) -> QueryUnderstanding: 
       raise NotImplementedError 

class SystemQueryUnderstandingService(QueryUnderstandingService):
    async def understand(self, query: str) -> QueryUnderstanding:
        entities = self._extract_entities(query)
        intent = self._classify_intent(query)

        plan = ["keyword", "vector"]
        if entities or any(word in query.lower() for word in ["call", "calls", "import", "dependency", "depends"]):
            plan.append("graph")

        return QueryUnderstanding(intent=intent, entities=entities, retrieval_plan=plan)

    def _classify_intent(self, query: str) -> str:
        lowered = query.lower()
        if any(word in lowered for word in ["where", "find", "located", "defined"]):
            return "code_search"
        if any(word in lowered for word in ["why", "bug", "error", "fail", "fix"]):
            return "debugging"
        if any(word in lowered for word in ["explain", "how does", "what does"]):
            return "code_explanation"
        return "general_code_question"

    def _extract_entities(self, query: str) -> list[str]:
        file_paths = re.findall(r"[\w./-]+\.[A-Za-z0-9]+", query)
        symbols = re.findall(r"`([^`]+)`", query)
        return sorted(set(file_paths + symbols))


class LLMQueryUnderstandingService(QueryUnderstandingService):
    def __init__(self, llm_client: LLMClient) -> None:
        self.llm_client = llm_client
        self.fallback_service = SystemQueryUnderstandingService()

    async def understand(self, query: str) -> QueryUnderstanding:
        prompt = """
You are a query understanding engine for a codebase RAG system.

Your job is to understand the user's code-related query at a high level.
You do NOT generate subqueries.
You do NOT answer the question.
You do NOT invent facts about the repository.

Return only valid JSON in exactly this format:

{
  "intent": "code_search | debugging | code_explanation | execution_flow | dependency_analysis | configuration | general_code_question",
  "entities": ["string"],
  "retrieval_plan": ["keyword", "vector", "graph"]
}

Field meanings:
- intent: the primary type of information need in the user query
- entities: exact important identifiers or concrete repo-specific terms explicitly present in the query, such as file names, class names, function names, symbols, paths, error strings, or backticked terms
- retrieval_plan: the minimal useful set of retrieval strategies needed for this query

Available retrieval strategies:
- keyword: use for exact identifiers, filenames, symbols, paths, repo-specific terms, error messages, stack traces, and precise matching
- vector: use for semantic understanding, conceptual questions, behavior, explanations, and fuzzy matching
- graph: use for relationships such as call chains, imports, dependencies, inheritance, execution flow, or data flow

Rules:
- Return only JSON
- Do not include markdown
- Do not include explanations outside the JSON
- Preserve exact important user terms in entities whenever possible
- Use the smallest sufficient retrieval_plan
- Include graph only when relationships or structure are likely important

Examples:

User query:
Where is `AuthMiddleware` defined?

Output:
{
  "intent": "code_search",
  "entities": ["AuthMiddleware"],
  "retrieval_plan": ["keyword", "vector"]
}

User query:
How does a request get authenticated before hitting the handler?

Output:
{
  "intent": "execution_flow",
  "entities": ["request", "handler"],
  "retrieval_plan": ["keyword", "vector", "graph"]
}

User query:
Why does `ChunkAssembler` fail when the repo has deleted files?

Output:
{
  "intent": "debugging",
  "entities": ["ChunkAssembler", "deleted files"],
  "retrieval_plan": ["keyword", "vector", "graph"]
}

Now analyze the following user query and return JSON only.

User query:
{query}
""".strip().format(query=query)

        try:
            response = await self.llm_client.complete(prompt)
            data = json.loads(self._extract_json(response))

            return QueryUnderstanding(
                intent=data["intent"],
                entities=data.get("entities", []),
                retrieval_plan=data.get("retrieval_plan", ["keyword", "vector"]),
            )
        except Exception:
            return self.fallback_service.understand(query)

    def _extract_json(self, text: str) -> str:
        text = text.strip()

        if text.startswith("```"):
            lines = text.splitlines()
            if len(lines) >= 3:
                return "\n".join(lines[1:-1]).strip()

        return text
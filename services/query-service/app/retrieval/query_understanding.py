import re

from app.models.retrieval import QueryUnderstanding


class QueryUnderstandingService:
    def understand(self, query: str) -> QueryUnderstanding:
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

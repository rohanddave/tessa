from app.context.budget import ContextBudget
from app.context.dedupe import dedupe_hits
from app.models.context import AssembledContext, ContextBlock
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalResult


class ContextAssemblyService:
    async def assemble(self, request: QueryRequest, retrieval_result: RetrievalResult) -> AssembledContext:
        budget = ContextBudget(request.context_token_budget)
        blocks: list[ContextBlock] = []

        for hit in dedupe_hits(retrieval_result.hits):
            content = hit.content or ""
            if not content or not budget.can_add(content):
                continue

            budget.add(content)
            blocks.append(
                ContextBlock(
                    chunk_id=hit.chunk_id,
                    file_path=hit.file_path,
                    language=hit.language,
                    symbol_name=hit.symbol_name,
                    start_line=hit.start_line,
                    end_line=hit.end_line,
                    content=content,
                    score=hit.score,
                    sources=hit.sources,
                )
            )

        return AssembledContext(
            repo_url=request.repo_url,
            branch=request.branch,
            snapshot_id=request.snapshot_id,
            query=request.query,
            token_budget=request.context_token_budget,
            blocks=blocks,
        )

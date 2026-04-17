import logging
from collections import Counter

from app.context.budget import ContextBudget
from app.context.dedupe import dedupe_hits
from app.models.context import AssembledContext, ContextBlock
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalResult


class ContextAssemblyService:
    def __init__(self, logger: logging.Logger | None = None) -> None:
        self.logger = logger or logging.getLogger(__name__)

    async def assemble(self, request: QueryRequest, retrieval_result: RetrievalResult) -> AssembledContext:
        budget = ContextBudget(request.context_token_budget)
        blocks: list[ContextBlock] = []
        deduped_hits = dedupe_hits(retrieval_result.hits)
        skipped_empty = 0
        skipped_budget = 0
        source_counts: Counter[str] = Counter()

        for hit in deduped_hits:
            content = hit.content or ""
            if not content:
                skipped_empty += 1
                continue
            if not budget.can_add(content):
                skipped_budget += 1
                continue

            budget.add(content)
            source_counts.update(hit.sources)
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

        self.logger.info(
            "context assembly completed input_hits=%d deduped_hits=%d blocks=%d skipped_empty=%d skipped_budget=%d budget_tokens=%d used_estimated_tokens=%d remaining_estimated_tokens=%d source_counts=%s",
            len(retrieval_result.hits),
            len(deduped_hits),
            len(blocks),
            skipped_empty,
            skipped_budget,
            request.context_token_budget,
            budget.used_estimated_tokens,
            max(0, request.context_token_budget - budget.used_estimated_tokens),
            dict(source_counts),
        )

        return AssembledContext(
            repo_url=request.repo_url,
            branch=request.branch,
            snapshot_id=request.snapshot_id,
            query=request.query,
            token_budget=request.context_token_budget,
            blocks=blocks,
        )

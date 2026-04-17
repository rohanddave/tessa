from collections import defaultdict

from app.models.retrieval import RetrievalHit


class FusionRanker:
    def fuse(self, result_sets: list[list[RetrievalHit]], limit: int) -> list[RetrievalHit]:
        by_chunk: dict[str, RetrievalHit] = {}
        scores: dict[str, float] = defaultdict(float)

        for hits in result_sets:
            for rank, hit in enumerate(hits, start=1):
                by_chunk.setdefault(hit.chunk_id, hit)
                scores[hit.chunk_id] += 1 / (60 + rank)

                existing = by_chunk[hit.chunk_id]
                self._merge_missing_fields(existing, hit)
                existing.sources = sorted(set(existing.sources + hit.sources))
                existing.score = max(existing.score, hit.score)

        fused = list(by_chunk.values())
        fused.sort(key=lambda hit: (scores[hit.chunk_id], hit.score), reverse=True)
        return fused[:limit]

    def _merge_missing_fields(self, existing: RetrievalHit, candidate: RetrievalHit) -> None:
        for field in [
            "repo_url",
            "branch",
            "snapshot_id",
            "file_path",
            "language",
            "symbol_name",
            "symbol_type",
            "start_line",
            "end_line",
            "content",
            "prev_chunk_id",
            "next_chunk_id",
        ]:
            if getattr(existing, field) is None and getattr(candidate, field) is not None:
                setattr(existing, field, getattr(candidate, field))

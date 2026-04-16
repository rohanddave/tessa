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
                existing.sources = sorted(set(existing.sources + hit.sources))
                existing.score = max(existing.score, hit.score)

        fused = list(by_chunk.values())
        fused.sort(key=lambda hit: (scores[hit.chunk_id], hit.score), reverse=True)
        return fused[:limit]

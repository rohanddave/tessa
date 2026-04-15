from app.models.retrieval import RetrievalHit


def dedupe_hits(hits: list[RetrievalHit]) -> list[RetrievalHit]:
    seen: set[str] = set()
    deduped: list[RetrievalHit] = []

    for hit in hits:
        if hit.chunk_id in seen:
            continue
        seen.add(hit.chunk_id)
        deduped.append(hit)

    return deduped

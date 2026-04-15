from __future__ import annotations

from typing import Any

from app.models.retrieval import RetrievalHit


def retrieval_hit_from_chunk(
    chunk: dict[str, Any],
    *,
    score: float = 0,
    source: str,
) -> RetrievalHit | None:
    chunk_id = str(chunk.get("chunk_id") or "").strip()
    if not chunk_id:
        return None

    return RetrievalHit(
        chunk_id=chunk_id,
        repo_url=optional_string(chunk.get("repo_url")),
        branch=optional_string(chunk.get("branch")),
        snapshot_id=optional_string(chunk.get("snapshot_id")),
        file_path=optional_string(chunk.get("file_path")),
        language=optional_string(chunk.get("language")),
        symbol_name=optional_string(chunk.get("symbol_name")),
        symbol_type=optional_string(chunk.get("symbol_type")),
        start_line=optional_int(chunk.get("start_line")),
        end_line=optional_int(chunk.get("end_line")),
        content=optional_string(chunk.get("content")),
        prev_chunk_id=optional_string(chunk.get("prev_chunk_id")),
        next_chunk_id=optional_string(chunk.get("next_chunk_id")),
        score=score,
        sources=[source],
    )


def optional_string(value: Any) -> str | None:
    if value is None:
        return None
    text = str(value)
    return text if text else None


def optional_int(value: Any) -> int | None:
    if value is None or value == "":
        return None
    return int(value)

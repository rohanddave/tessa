from __future__ import annotations

from typing import Any

from app.config import Settings
from app.models.chunk import retrieval_hit_from_chunk
from app.models.retrieval import RetrievalHit

try:
    import asyncpg
except ImportError:  # pragma: no cover - exercised only when optional runtime dep is absent.
    asyncpg = None  # type: ignore[assignment]


class PostgresStore:
    def __init__(self, settings: Settings) -> None:
        self.host = settings.snapshot_store_host
        self.port = settings.snapshot_store_port
        self.database = settings.snapshot_store_db
        self.user = settings.snapshot_store_user
        self.password = settings.snapshot_store_password
        self.sslmode = settings.snapshot_store_sslmode

    async def mget_chunks(self, chunk_ids: list[str], *, source: str = "postgres") -> list[RetrievalHit]:
        ids = [chunk_id for chunk_id in dict.fromkeys(chunk_ids) if chunk_id]
        if not ids or asyncpg is None:
            return []

        conn = await asyncpg.connect(
            host=self.host,
            port=self.port,
            database=self.database,
            user=self.user,
            password=self.password,
            ssl=self._ssl(),
        )
        try:
            rows = await conn.fetch(
                """
                SELECT
                    chunk_id,
                    repo_url,
                    file_name,
                    branch,
                    commit_sha,
                    snapshot_id,
                    file_path,
                    document_type,
                    language,
                    content,
                    content_hash,
                    symbol_name,
                    symbol_type,
                    start_line,
                    end_line,
                    start_char,
                    end_char,
                    prev_chunk_id,
                    next_chunk_id,
                    status,
                    elasticsearch_status,
                    pinecone_status,
                    neo4j_status,
                    created_at
                FROM chunks
                WHERE chunk_id = ANY($1::text[])
                """,
                ids,
            )
        finally:
            await conn.close()

        by_id: dict[str, RetrievalHit] = {}
        for row in rows:
            hit = retrieval_hit_from_chunk(dict(row), score=0, source=source)
            if hit is not None:
                by_id[hit.chunk_id] = hit

        return [by_id[chunk_id] for chunk_id in ids if chunk_id in by_id]

    def _ssl(self) -> bool | str:
        if self.sslmode in {"disable", "allow", "prefer"}:
            return False
        return self.sslmode

from __future__ import annotations

import re
import logging
import time
from typing import Any

from app.config import Settings
from app.models.chunk import retrieval_hit_from_chunk
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit

try:
    from neo4j import AsyncGraphDatabase
except ImportError:  # pragma: no cover - exercised only when optional runtime dep is absent.
    AsyncGraphDatabase = None  # type: ignore[assignment]


class Neo4jStore:
    def __init__(self, settings: Settings) -> None:
        self.uri = settings.neo4j_uri
        self.user = settings.neo4j_user
        self.password = settings.neo4j_password
        self.logger = logging.getLogger(settings.service_name)

    async def search(
        self,
        request: QueryRequest,
        entities: list[str],
        *,
        top_k: int,
    ) -> list[RetrievalHit]:
        if AsyncGraphDatabase is None:
            self.logger.warning("neo4j driver unavailable; graph retrieval skipped")
            return []

        terms = self._search_terms(request.query, entities)
        if not terms:
            self.logger.info("graph retrieval skipped terms=0")
            return []

        started = time.monotonic()
        self.logger.info(
            "graph retrieval started terms=%d top_k=%d repo_scoped=%s snapshot_scoped=%s",
            len(terms),
            top_k,
            bool(request.repo_url),
            bool(request.snapshot_id),
        )
        driver = AsyncGraphDatabase.driver(self.uri, auth=(self.user, self.password))
        try:
            async with driver.session() as session:
                result = await session.run(
                    graph_retrieval_query(),
                    {
                        "repo_url": request.repo_url,
                        "branch": request.branch,
                        "snapshot_id": request.snapshot_id,
                        "terms": terms,
                        "anchor_limit": max(top_k, 4),
                        "limit": top_k,
                    },
                )

                hits: list[RetrievalHit] = []
                async for record in result:
                    hit = retrieval_hit_from_chunk(
                        dict(record["chunk"]),
                        score=float(record["score"] or 0),
                        source="graph",
                    )
                    if hit is not None:
                        hits.append(hit)
                self.logger.info(
                    "graph retrieval completed hits=%d duration_ms=%.2f",
                    len(hits),
                    (time.monotonic() - started) * 1000,
                )
                return hits
        finally:
            await driver.close()

    def _search_terms(self, query: str, entities: list[str]) -> list[str]:
        raw_terms = [*entities]
        raw_terms.extend(re.findall(r"`([^`]+)`", query))
        raw_terms.extend(re.findall(r"[\w./:-]+\.[A-Za-z0-9]+", query))
        raw_terms.extend(re.findall(r"\b[A-Z][A-Za-z0-9_]*\b", query))
        raw_terms.extend(re.findall(r"\b[a-zA-Z_][A-Za-z0-9_]*(?:\(\))?", query))

        terms: list[str] = []
        for term in raw_terms:
            normalized = term.strip().strip("`").removesuffix("()").lower()
            if len(normalized) < 3:
                continue
            if normalized in stop_words():
                continue
            if normalized not in terms:
                terms.append(normalized)
        return terms[:12]


def stop_words() -> set[str]:
    return {
        "about",
        "call",
        "calls",
        "code",
        "depend",
        "depends",
        "dependency",
        "does",
        "explain",
        "find",
        "from",
        "how",
        "import",
        "imports",
        "where",
        "what",
    }


def graph_retrieval_query() -> str:
    return """
    MATCH (anchor:Chunk)
    WHERE anchor.repo_url IS NOT NULL
      AND coalesce(anchor.synthetic, false) = false
      AND ($repo_url IS NULL OR anchor.repo_url = $repo_url)
      AND ($branch IS NULL OR anchor.branch = $branch)
      AND ($snapshot_id IS NULL OR anchor.snapshot_id = $snapshot_id)
      AND any(term IN $terms WHERE
        toLower(coalesce(anchor.symbol_name, "")) CONTAINS term OR
        toLower(coalesce(anchor.file_path, "")) CONTAINS term OR
        toLower(coalesce(anchor.symbol_type, "")) CONTAINS term
      )
    WITH anchor
    ORDER BY
      CASE
        WHEN any(term IN $terms WHERE toLower(coalesce(anchor.symbol_name, "")) = term) THEN 0
        WHEN any(term IN $terms WHERE toLower(coalesce(anchor.file_path, "")) CONTAINS term) THEN 1
        ELSE 2
      END,
      anchor.start_char ASC
    LIMIT $anchor_limit

    CALL {
      WITH anchor
      RETURN anchor AS chunk, 1.0 AS score

      UNION

      WITH anchor
      MATCH (anchor)-[r:PREV|NEXT|CONTAINS|HAS_IMPORT|RESOLVES_TO_FILE|DOCUMENTS|ALIAS_OF]-(chunk:Chunk)
      WHERE chunk.repo_url IS NOT NULL
        AND coalesce(chunk.synthetic, false) = false
      RETURN chunk,
        CASE type(r)
          WHEN "DOCUMENTS" THEN 0.82
          WHEN "CONTAINS" THEN 0.78
          WHEN "RESOLVES_TO_FILE" THEN 0.72
          WHEN "HAS_IMPORT" THEN 0.66
          WHEN "ALIAS_OF" THEN 0.62
          ELSE 0.55
        END AS score

      UNION

      WITH anchor
      MATCH (anchor)-[:HAS_IMPORT|RESOLVES_TO_FILE]-(file:Chunk {symbol_type: "file"})-[:CONTAINS]->(chunk:Chunk)
      WHERE chunk.repo_url IS NOT NULL
        AND coalesce(chunk.synthetic, false) = false
      RETURN chunk, 0.58 AS score

      UNION

      WITH anchor
      MATCH (anchor)-[:CONTAINS]-(mid:Chunk)-[:CONTAINS]-(chunk:Chunk)
      WHERE chunk.repo_url IS NOT NULL
        AND coalesce(chunk.synthetic, false) = false
      RETURN chunk, 0.45 AS score
    }

    WITH chunk, max(score) AS score
    RETURN chunk {
      .chunk_id,
      .repo_url,
      .branch,
      .snapshot_id,
      .file_path,
      .language,
      .symbol_name,
      .symbol_type,
      .start_line,
      .end_line,
      .prev_chunk_id,
      .next_chunk_id
    } AS chunk, score
    ORDER BY score DESC, chunk.file_path ASC, chunk.start_line ASC
    LIMIT $limit
    """

import asyncio

from app.models.chunk import retrieval_hit_from_chunk
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalHit
from app.retrieval.context_expansion import ContextExpansionService


class FakeElasticsearch:
    async def mget(self, chunk_ids: list[str], *, source: str = "context_expansion") -> list[RetrievalHit]:
        chunks = {
            "prev": RetrievalHit(
                chunk_id="prev",
                repo_url="repo",
                branch="main",
                snapshot_id="snap",
                content="before",
                sources=[source],
            ),
            "next": RetrievalHit(
                chunk_id="next",
                repo_url="repo",
                branch="main",
                snapshot_id="snap",
                content="after",
                sources=[source],
            ),
        }
        return [chunks[chunk_id] for chunk_id in chunk_ids if chunk_id in chunks]


def test_retrieval_hit_from_chunk_maps_full_context() -> None:
    hit = retrieval_hit_from_chunk(
        {
            "chunk_id": "chunk-1",
            "repo_url": "repo",
            "branch": "main",
            "snapshot_id": "snap",
            "file_path": "app/main.py",
            "language": "python",
            "symbol_name": "answer",
            "symbol_type": "function",
            "start_line": 10,
            "end_line": 20,
            "content": "def answer(): pass",
            "prev_chunk_id": "prev",
            "next_chunk_id": "next",
        },
        score=1.5,
        source="keyword",
    )

    assert hit is not None
    assert hit.chunk_id == "chunk-1"
    assert hit.content == "def answer(): pass"
    assert hit.prev_chunk_id == "prev"
    assert hit.next_chunk_id == "next"
    assert hit.sources == ["keyword"]


def test_context_expansion_includes_neighbor_chunks() -> None:
    expanded = asyncio.run(expand_context())

    assert [hit.chunk_id for hit in expanded] == ["prev", "current", "next"]


async def expand_context() -> list[RetrievalHit]:
    service = ContextExpansionService(FakeElasticsearch())  # type: ignore[arg-type]
    request = QueryRequest(query="explain answer", repo_url="repo", branch="main", snapshot_id="snap")

    return await service.expand(
        request,
        [
            RetrievalHit(
                chunk_id="current",
                repo_url="repo",
                branch="main",
                snapshot_id="snap",
                content="current",
                prev_chunk_id="prev",
                next_chunk_id="next",
            )
        ],
    )

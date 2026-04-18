import asyncio
import logging

from app.models.chunk import retrieval_hit_from_chunk
from app.models.query import QueryRequest
from app.models.retrieval import QueryUnderstanding, RetrievalHit
from app.retrieval.context_expansion import ContextExpansionService
from app.retrieval.fusion import FusionRanker
from app.retrieval.orchestrator import RetrievalOrchestrator
from app.retrieval.retrievers.graph import GraphRetriever


class FakePostgresForExpansion:
    async def mget_chunks(self, chunk_ids: list[str], *, source: str = "context_expansion") -> list[RetrievalHit]:
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


def test_graph_retriever_hydrates_chunk_content() -> None:
    hits = asyncio.run(retrieve_graph_hits())

    assert [hit.chunk_id for hit in hits] == ["graph-1"]
    assert hits[0].content == "hydrated content"
    assert hits[0].score == 0.9
    assert hits[0].sources == ["graph"]


def test_fusion_merges_content_from_later_duplicate_hit() -> None:
    fused = FusionRanker().fuse(
        [
            [RetrievalHit(chunk_id="same", score=0.9, sources=["graph"])],
            [RetrievalHit(chunk_id="same", content="full text", score=0.5, sources=["keyword"])],
        ],
        limit=5,
    )

    assert len(fused) == 1
    assert fused[0].content == "full text"
    assert fused[0].score == 0.9
    assert fused[0].sources == ["graph", "keyword"]


def test_retrieval_orchestrator_stores_and_reuses_stateful_chunks() -> None:
    result = asyncio.run(orchestrate_stateful_retrieval())

    assert [hit.chunk_id for hit in result.hits] == ["current", "next"]


async def expand_context() -> list[RetrievalHit]:
    service = ContextExpansionService(FakePostgresForExpansion())  # type: ignore[arg-type]
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


async def retrieve_graph_hits() -> list[RetrievalHit]:
    request = QueryRequest(query="where is `InvoiceService` used?", repo_url="repo", branch="main", snapshot_id="snap")
    retriever = GraphRetriever(FakeNeo4j(), FakePostgres())  # type: ignore[arg-type]
    return await retriever.retrieve(
        request,
        understanding=type("Understanding", (), {"entities": ["InvoiceService"]})(),
    )


async def orchestrate_stateful_retrieval():
    orchestrator = RetrievalOrchestrator(
        logging.getLogger(__name__),
        FakeQueryUnderstanding(),
        retrievers=[FakeRetriever()],
        context_expansion=ContextExpansionService(FakePostgresForExpansion()),  # type: ignore[arg-type]
    )
    request = QueryRequest(query="explain answer", repo_url="repo", branch="main", snapshot_id="snap")

    result = await orchestrator.retrieve(request)

    assert len(orchestrator.chunks) == 1
    assert [hit.chunk_id for hit in orchestrator.chunks[0]] == ["current"]
    assert [hit.chunk_id for hit in orchestrator.fused_chunks] == ["current"]
    assert [hit.chunk_id for hit in orchestrator.expanded_chunks] == ["current", "next"]
    return result


class FakeNeo4j:
    async def search(self, request: QueryRequest, entities: list[str], *, top_k: int) -> list[RetrievalHit]:
        assert entities == ["InvoiceService"]
        return [
            RetrievalHit(
                chunk_id="graph-1",
                repo_url="repo",
                branch="main",
                snapshot_id="snap",
                score=0.9,
                sources=["graph"],
            )
        ]


class FakePostgres:
    async def mget_chunks(self, chunk_ids: list[str], *, source: str = "postgres") -> list[RetrievalHit]:
        assert chunk_ids == ["graph-1"]
        assert source == "graph"
        return [
            RetrievalHit(
                chunk_id="graph-1",
                repo_url="repo",
                branch="main",
                snapshot_id="snap",
                content="hydrated content",
                score=0,
                sources=[source],
            )
        ]


class FakeQueryUnderstanding:
    async def understand(self, query: str) -> QueryUnderstanding:
        assert query == "explain answer"
        return QueryUnderstanding(
            intent="code_explanation",
            entities=["answer"],
            retrieval_plan=["fake"],
        )


class FakeRetriever:
    name = "fake"

    async def retrieve(self, request: QueryRequest, understanding: QueryUnderstanding) -> list[RetrievalHit]:
        assert understanding.retrieval_plan == ["fake"]
        return [
            RetrievalHit(
                chunk_id="current",
                repo_url=request.repo_url,
                branch=request.branch,
                snapshot_id=request.snapshot_id,
                content="current",
                next_chunk_id="next",
                score=0.9,
                sources=[self.name],
            )
        ]

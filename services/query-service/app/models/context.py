from pydantic import BaseModel, Field


class ContextBlock(BaseModel):
    chunk_id: str
    file_path: str | None = None
    language: str | None = None
    symbol_name: str | None = None
    start_line: int | None = None
    end_line: int | None = None
    content: str
    score: float = 0
    sources: list[str] = Field(default_factory=list)


class AssembledContext(BaseModel):
    repo_url: str | None = None
    branch: str = "main"
    snapshot_id: str | None = None
    query: str
    token_budget: int
    blocks: list[ContextBlock] = Field(default_factory=list)

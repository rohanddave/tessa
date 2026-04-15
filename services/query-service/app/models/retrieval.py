from pydantic import BaseModel, Field


class QueryUnderstanding(BaseModel):
    intent: str = "general_code_question"
    entities: list[str] = Field(default_factory=list)
    retrieval_plan: list[str] = Field(default_factory=list)


class RetrievalHit(BaseModel):
    chunk_id: str
    repo_url: str | None = None
    branch: str | None = None
    snapshot_id: str | None = None
    file_path: str | None = None
    language: str | None = None
    symbol_name: str | None = None
    symbol_type: str | None = None
    start_line: int | None = None
    end_line: int | None = None
    content: str | None = None
    score: float = 0
    sources: list[str] = Field(default_factory=list)


class RetrievalResult(BaseModel):
    understanding: QueryUnderstanding
    hits: list[RetrievalHit] = Field(default_factory=list)


class RetrievalResponse(BaseModel):
    query: str
    result: RetrievalResult

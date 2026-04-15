from pydantic import BaseModel, Field

from app.models.context import ContextBlock
from app.models.query import QueryRequest


class AnswerRequest(QueryRequest):
    stream: bool = False


class Citation(BaseModel):
    chunk_id: str
    file_path: str | None = None
    start_line: int | None = None
    end_line: int | None = None


class AnswerResponse(BaseModel):
    answer: str
    citations: list[Citation] = Field(default_factory=list)
    context_blocks: list[ContextBlock] = Field(default_factory=list)
    limitations: list[str] = Field(default_factory=list)

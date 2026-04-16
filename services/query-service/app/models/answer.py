from __future__ import annotations

from typing import List, Optional

from pydantic import BaseModel, Field

from app.models.context import ContextBlock
from app.models.query import QueryRequest


class AnswerRequest(QueryRequest):
    stream: bool = False


class Citation(BaseModel):
    chunk_id: str
    file_path: Optional[str] = None
    start_line: Optional[int] = None
    end_line: Optional[int] = None


class AnswerResponse(BaseModel):
    answer: str
    citations: List[Citation] = Field(default_factory=list)
    context_blocks: List[ContextBlock] = Field(default_factory=list)
    limitations: List[str] = Field(default_factory=list)

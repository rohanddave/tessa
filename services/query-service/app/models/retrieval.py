from __future__ import annotations

from typing import List, Optional

from pydantic import BaseModel, Field


class QueryUnderstanding(BaseModel):
    intent: str = "general_code_question"
    entities: List[str] = Field(default_factory=list)
    retrieval_plan: List[str] = Field(default_factory=list)


class RetrievalHit(BaseModel):
    chunk_id: str
    repo_url: Optional[str] = None
    branch: Optional[str] = None
    snapshot_id: Optional[str] = None
    file_path: Optional[str] = None
    language: Optional[str] = None
    symbol_name: Optional[str] = None
    symbol_type: Optional[str] = None
    start_line: Optional[int] = None
    end_line: Optional[int] = None
    content: Optional[str] = None
    prev_chunk_id: Optional[str] = None
    next_chunk_id: Optional[str] = None
    score: float = 0
    sources: List[str] = Field(default_factory=list)


class RetrievalResult(BaseModel):
    understanding: QueryUnderstanding
    hits: List[RetrievalHit] = Field(default_factory=list)


class RetrievalResponse(BaseModel):
    query: str
    result: RetrievalResult

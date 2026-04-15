from __future__ import annotations

from typing import List, Optional

from pydantic import BaseModel, Field


class ContextBlock(BaseModel):
    chunk_id: str
    file_path: Optional[str] = None
    language: Optional[str] = None
    symbol_name: Optional[str] = None
    start_line: Optional[int] = None
    end_line: Optional[int] = None
    content: str
    score: float = 0
    sources: List[str] = Field(default_factory=list)


class AssembledContext(BaseModel):
    repo_url: Optional[str] = None
    branch: str = "main"
    snapshot_id: Optional[str] = None
    query: str
    token_budget: int
    blocks: List[ContextBlock] = Field(default_factory=list)

from __future__ import annotations

from typing import Optional

from pydantic import BaseModel, Field


class QueryRequest(BaseModel):
    query: str = Field(..., min_length=1)
    repo_url: Optional[str] = None
    branch: str = "main"
    snapshot_id: Optional[str] = None
    top_k: int = Field(default=8, ge=1, le=50)
    context_token_budget: int = Field(default=12000, ge=1000, le=50000)

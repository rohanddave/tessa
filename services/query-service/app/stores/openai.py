from __future__ import annotations

from typing import Any

import httpx

from app.config import Settings


class OpenAIEmbeddingStore:
    def __init__(self, settings: Settings) -> None:
        self.api_key = settings.openai_api_key
        self.base_url = settings.openai_base_url
        self.model = settings.openai_embedding_model
        self.dimensions = settings.openai_embedding_dimensions

    async def embed(self, text: str) -> list[float]:
        if not self.api_key or not text.strip():
            return []

        body: dict[str, Any] = {
            "model": self.model,
            "input": text,
        }
        if self.dimensions > 0:
            body["dimensions"] = self.dimensions

        async with httpx.AsyncClient(timeout=20) as client:
            response = await client.post(
                f"{self.base_url.rstrip('/')}/embeddings",
                json=body,
                headers={
                    "Authorization": f"Bearer {self.api_key}",
                    "Content-Type": "application/json",
                },
            )
            response.raise_for_status()
            data = response.json()

        embeddings = data.get("data") or []
        if not embeddings:
            return []
        return embeddings[0].get("embedding") or []

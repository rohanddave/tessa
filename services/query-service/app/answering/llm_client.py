from __future__ import annotations

from typing import Any

import httpx

from app.config import Settings


class LLMClient:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings

    async def complete(self, prompt: str) -> str:
        if not self.settings.openai_api_key:
            return "LLM answering is not configured yet. Set OPENAI_API_KEY to enable generated answers."

        body = {
            "model": self.settings.openai_answer_model,
            "instructions": (
                "You answer questions about a code repository. Use only the supplied context. "
                "When the context is insufficient, say what is missing instead of guessing."
            ),
            "input": prompt,
            "max_output_tokens": self.settings.openai_answer_max_output_tokens,
        }

        async with httpx.AsyncClient(timeout=self.settings.openai_timeout_seconds) as client:
            response = await client.post(
                f"{self.settings.openai_base_url.rstrip('/')}/responses",
                json=body,
                headers={
                    "Authorization": f"Bearer {self.settings.openai_api_key}",
                    "Content-Type": "application/json",
                },
            )

        if response.status_code < 200 or response.status_code >= 300:
            raise LLMClientError(f"OpenAI Responses API request failed: {response.status_code} {response.text}")

        return self._extract_text(response.json())

    def _extract_text(self, payload: dict[str, Any]) -> str:
        output_text = payload.get("output_text")
        if isinstance(output_text, str) and output_text.strip():
            return output_text.strip()

        parts: list[str] = []
        for item in payload.get("output") or []:
            for content in item.get("content") or []:
                text = content.get("text")
                if isinstance(text, str) and text:
                    parts.append(text)

        answer = "\n".join(parts).strip()
        if not answer:
            raise LLMClientError("OpenAI Responses API response did not include output text")
        return answer


class LLMClientError(RuntimeError):
    pass

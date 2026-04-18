from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import httpx

from app.config import Settings
from app.models.answer import TokenUsage


@dataclass(frozen=True)
class CompletionResult:
    text: str
    token_usage: TokenUsage


class LLMClient:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings
        self._token_usage = TokenUsage(input_tokens=0, output_tokens=0, total_tokens=0)

    async def complete(self, prompt: str) -> str:
        result = await self.complete_with_usage(prompt)
        return result.text

    async def complete_with_usage(self, prompt: str) -> CompletionResult:
        if not self.settings.openai_api_key:
            result = CompletionResult(
                text="LLM answering is not configured yet. Set OPENAI_API_KEY to enable generated answers.",
                token_usage=TokenUsage(input_tokens=0, output_tokens=0, total_tokens=0),
            )
            self._record_token_usage(result.token_usage)
            return result

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

        payload = response.json()
        result = CompletionResult(
            text=self._extract_text(payload),
            token_usage=self._extract_token_usage(payload),
        )
        self._record_token_usage(result.token_usage)
        return result

    def reset_token_usage(self) -> None:
        self._token_usage = TokenUsage(input_tokens=0, output_tokens=0, total_tokens=0)

    def token_usage_snapshot(self) -> TokenUsage:
        return self._token_usage.model_copy()

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

    def _extract_token_usage(self, payload: dict[str, Any]) -> TokenUsage:
        usage = payload.get("usage")
        if not isinstance(usage, dict):
            return TokenUsage()

        input_tokens = self._optional_int(usage.get("input_tokens"))
        output_tokens = self._optional_int(usage.get("output_tokens"))
        total_tokens = self._optional_int(usage.get("total_tokens"))
        if total_tokens is None and input_tokens is not None and output_tokens is not None:
            total_tokens = input_tokens + output_tokens

        return TokenUsage(
            input_tokens=input_tokens,
            output_tokens=output_tokens,
            total_tokens=total_tokens,
        )

    def _optional_int(self, value: Any) -> int | None:
        if isinstance(value, bool):
            return None
        if isinstance(value, int):
            return value
        return None

    def _record_token_usage(self, token_usage: TokenUsage) -> None:
        self._token_usage = TokenUsage(
            input_tokens=(self._token_usage.input_tokens or 0) + (token_usage.input_tokens or 0),
            output_tokens=(self._token_usage.output_tokens or 0) + (token_usage.output_tokens or 0),
            total_tokens=(self._token_usage.total_tokens or 0) + (token_usage.total_tokens or 0),
        )


class LLMClientError(RuntimeError):
    pass

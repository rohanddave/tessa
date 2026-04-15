import asyncio

import httpx

from app.answering.llm_client import LLMClient, LLMClientError
from app.config import Settings


def test_complete_returns_missing_api_key_message() -> None:
    client = LLMClient(Settings(openai_api_key=""))

    answer = asyncio.run(client.complete("hello"))

    assert "OPENAI_API_KEY" in answer


def test_extract_text_supports_output_text() -> None:
    client = LLMClient(Settings())

    assert client._extract_text({"output_text": " hello "}) == "hello"


def test_extract_text_supports_structured_output() -> None:
    client = LLMClient(Settings())

    answer = client._extract_text(
        {
            "output": [
                {
                    "content": [
                        {"type": "output_text", "text": "first"},
                        {"type": "output_text", "text": "second"},
                    ]
                }
            ]
        }
    )

    assert answer == "first\nsecond"


def test_extract_text_errors_when_empty() -> None:
    client = LLMClient(Settings())

    try:
        client._extract_text({"output": []})
    except LLMClientError as err:
        assert "output text" in str(err)
    else:
        raise AssertionError("expected LLMClientError")


def test_complete_posts_to_responses_api() -> None:
    captured = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["url"] = str(request.url)
        captured["authorization"] = request.headers.get("Authorization")
        captured["body"] = request.read().decode()
        return httpx.Response(200, json={"output_text": "done"})

    transport = httpx.MockTransport(handler)
    original_client = httpx.AsyncClient

    def fake_async_client(*args, **kwargs):
        kwargs["transport"] = transport
        return original_client(*args, **kwargs)

    httpx.AsyncClient = fake_async_client  # type: ignore[method-assign]
    try:
        client = LLMClient(
            Settings(
                openai_api_key="test-key",
                openai_base_url="https://api.openai.test/v1",
                openai_answer_model="test-model",
                openai_answer_max_output_tokens=123,
            )
        )

        answer = asyncio.run(client.complete("question"))
    finally:
        httpx.AsyncClient = original_client  # type: ignore[method-assign]

    assert answer == "done"
    assert captured["url"] == "https://api.openai.test/v1/responses"
    assert captured["authorization"] == "Bearer test-key"
    assert '"model":"test-model"' in captured["body"]
    assert '"max_output_tokens":123' in captured["body"]

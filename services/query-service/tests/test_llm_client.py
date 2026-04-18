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


def test_extract_token_usage_reads_responses_usage() -> None:
    client = LLMClient(Settings())

    usage = client._extract_token_usage(
        {
            "usage": {
                "input_tokens": 101,
                "output_tokens": 23,
                "total_tokens": 124,
            }
        }
    )

    assert usage.input_tokens == 101
    assert usage.output_tokens == 23
    assert usage.total_tokens == 124


def test_extract_token_usage_derives_total_when_missing() -> None:
    client = LLMClient(Settings())

    usage = client._extract_token_usage({"usage": {"input_tokens": 10, "output_tokens": 5}})

    assert usage.input_tokens == 10
    assert usage.output_tokens == 5
    assert usage.total_tokens == 15


def test_token_usage_snapshot_accumulates_and_resets() -> None:
    client = LLMClient(Settings())

    client._record_token_usage(client._extract_token_usage({"usage": {"input_tokens": 10, "output_tokens": 5}}))
    client._record_token_usage(client._extract_token_usage({"usage": {"input_tokens": 3, "output_tokens": 2}}))

    usage = client.token_usage_snapshot()
    assert usage.input_tokens == 13
    assert usage.output_tokens == 7
    assert usage.total_tokens == 20

    client.reset_token_usage()
    usage = client.token_usage_snapshot()
    assert usage.input_tokens == 0
    assert usage.output_tokens == 0
    assert usage.total_tokens == 0


def test_complete_posts_to_responses_api() -> None:
    captured = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["url"] = str(request.url)
        captured["authorization"] = request.headers.get("Authorization")
        captured["body"] = request.read().decode()
        return httpx.Response(
            200,
            json={
                "output_text": "done",
                "usage": {
                    "input_tokens": 11,
                    "output_tokens": 7,
                    "total_tokens": 18,
                },
            },
        )

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


def test_complete_with_usage_returns_text_and_usage() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "output_text": "done",
                "usage": {
                    "input_tokens": 11,
                    "output_tokens": 7,
                    "total_tokens": 18,
                },
            },
        )

    transport = httpx.MockTransport(handler)
    original_client = httpx.AsyncClient

    def fake_async_client(*args, **kwargs):
        kwargs["transport"] = transport
        return original_client(*args, **kwargs)

    httpx.AsyncClient = fake_async_client  # type: ignore[method-assign]
    try:
        client = LLMClient(Settings(openai_api_key="test-key"))

        result = asyncio.run(client.complete_with_usage("question"))
    finally:
        httpx.AsyncClient = original_client  # type: ignore[method-assign]

    assert result.text == "done"
    assert result.token_usage.input_tokens == 11
    assert result.token_usage.output_tokens == 7
    assert result.token_usage.total_tokens == 18

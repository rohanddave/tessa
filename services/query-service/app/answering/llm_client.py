from app.config import Settings


class LLMClient:
    def __init__(self, settings: Settings) -> None:
        self.settings = settings

    async def complete(self, prompt: str) -> str:
        # TODO: Call OpenAI Responses API.
        if not self.settings.openai_api_key:
            return "LLM answering is not configured yet. Set OPENAI_API_KEY to enable generated answers."
        return "LLM client is scaffolded but not implemented yet."

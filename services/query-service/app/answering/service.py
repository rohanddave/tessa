from app.answering.llm_client import LLMClient, LLMClientError
from app.answering.prompts import build_answer_prompt
from app.context.citations import citations_from_context
from app.models.answer import AnswerResponse
from app.models.context import AssembledContext


class LLMAnsweringService:
    def __init__(self, llm_client: LLMClient) -> None:
        self.llm_client = llm_client

    async def answer(self, context: AssembledContext) -> AnswerResponse:
        if not context.blocks:
            return AnswerResponse(
                answer="I could not find relevant indexed context for that question yet.",
                limitations=["No context blocks were assembled from retrieval results."],
            )

        prompt = build_answer_prompt(context)
        try:
            completion = await self.llm_client.complete_with_usage(prompt)
        except LLMClientError as err:
            return AnswerResponse(
                answer="I found relevant context, but the LLM answer request failed.",
                citations=citations_from_context(context.blocks),
                context_blocks=context.blocks,
                limitations=[str(err)],
            )

        return AnswerResponse(
            answer=completion.text,
            citations=citations_from_context(context.blocks),
            context_blocks=context.blocks,
            token_usage=completion.token_usage,
        )

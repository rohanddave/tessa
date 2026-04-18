from __future__ import annotations

from fastapi import Depends, FastAPI

from app.config import Settings, get_settings
from app.models.answer import AnswerRequest, AnswerResponse, Mode
from app.models.query import QueryRequest
from app.models.retrieval import RetrievalResponse
from app.answering.baseline_strategy import BaselineRAGStrategy
from app.answering.reasoning_strategy import ReasoningRAGStrategy
from app.answering.agentic_strategy import AgenticRAGStrategy 

def build_app() -> FastAPI:
    app = FastAPI(title="Tessa Query Service", version="0.1.0")

    @app.get("/healthz")
    async def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.post("/retrieve", response_model=RetrievalResponse)
    async def retrieve(
        request: QueryRequest,
        services: QueryServices = Depends(get_query_services),
    ) -> RetrievalResponse:
        result = await services.retrieval.retrieve(request)
        return RetrievalResponse(query=request.query, result=result)

    @app.post("/answer", response_model=AnswerResponse)
    async def answer(
        request: AnswerRequest,
        services: QueryServices = Depends(get_query_services),
    ) -> AnswerResponse:
        if request.mode == Mode.BASELINE:
            return await services.baseline_strategy.answer(request)

        if request.mode == Mode.REASONING:
            return await services.reasoning_strategy.answer(request)

        if request.mode == Mode.AGENTIC:
            return await services.agentic_strategy.answer(request)
        
        return NotImplementedError

    return app


class QueryServices:
    def __init__(self, settings: Settings) -> None:
        self.baseline_strategy = BaselineRAGStrategy(settings)
        self.reasoning_strategy = ReasoningRAGStrategy(settings)
        self.agentic_strategy = AgenticRAGStrategy(settings)
        self.retrieval = self.baseline_strategy.retrieval

def get_query_services(settings: Settings = Depends(get_settings)) -> QueryServices:
    return QueryServices(settings)


app = build_app()

from app.models.answer import AnswerRequest, AnswerResponse

class AnsweringStrategy: 
    async def answer(self, request: AnswerRequest) -> AnswerResponse: 
        pass
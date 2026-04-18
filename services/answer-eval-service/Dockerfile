FROM python:3.12-slim

WORKDIR /app

COPY answer-eval-service/pyproject.toml ./
COPY answer-eval-service/app ./app
COPY answer-eval-service/questions ./questions
RUN pip install --no-cache-dir .

ENV PYTHONUNBUFFERED=1

CMD ["python", "-m", "app.main"]

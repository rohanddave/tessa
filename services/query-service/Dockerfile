FROM python:3.12-slim

WORKDIR /app

COPY query-service/pyproject.toml ./
COPY query-service/app ./app
RUN pip install --no-cache-dir .

ENV PYTHONUNBUFFERED=1

CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8082"]

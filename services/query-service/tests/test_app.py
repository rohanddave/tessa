from fastapi.testclient import TestClient

from app.main import app


def test_healthz() -> None:
    client = TestClient(app)

    response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_retrieve_scaffold() -> None:
    client = TestClient(app)

    response = client.post("/retrieve", json={"query": "where is indexing handled?"})

    assert response.status_code == 200
    body = response.json()
    assert body["query"] == "where is indexing handled?"
    assert body["result"]["understanding"]["intent"] == "code_search"
    assert body["result"]["hits"] == []

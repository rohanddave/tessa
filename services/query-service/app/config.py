from functools import lru_cache

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_file=".env", extra="ignore")

    service_name: str = "query-service"
    log_level: str = "info"
    port: int = 8082

    snapshot_store_host: str = "localhost"
    snapshot_store_port: int = 5432
    snapshot_store_db: str = "snapshot_store"
    snapshot_store_user: str = "postgres"
    snapshot_store_password: str = "postgres"
    snapshot_store_sslmode: str = "disable"

    elasticsearch_url: str = "http://localhost:9200"
    elasticsearch_index: str = "tessa-chunks"

    pinecone_host: str = "http://localhost:5081"
    pinecone_api_key: str = "pclocal"
    pinecone_api_version: str = "2025-01"
    pinecone_index: str = "tessa-chunks"
    pinecone_namespace: str = "__default__"

    neo4j_uri: str = "bolt://localhost:7687"
    neo4j_user: str = "neo4j"
    neo4j_password: str = "password"

    openai_api_key: str = ""
    openai_base_url: str = "https://api.openai.com/v1"
    openai_answer_model: str = "gpt-5.4-mini"
    openai_embedding_model: str = "text-embedding-3-small"
    openai_embedding_dimensions: int = 1536

    default_top_k: int = 8
    default_context_token_budget: int = 12000


@lru_cache
def get_settings() -> Settings:
    return Settings()

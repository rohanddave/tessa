from app.config import Settings


class Neo4jStore:
    def __init__(self, settings: Settings) -> None:
        self.uri = settings.neo4j_uri
        self.user = settings.neo4j_user

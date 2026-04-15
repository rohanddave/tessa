from app.config import Settings


class PineconeStore:
    def __init__(self, settings: Settings) -> None:
        self.host = settings.pinecone_host
        self.index = settings.pinecone_index
        self.namespace = settings.pinecone_namespace

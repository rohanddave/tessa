from app.config import Settings


class ElasticsearchStore:
    def __init__(self, settings: Settings) -> None:
        self.url = settings.elasticsearch_url
        self.index = settings.elasticsearch_index

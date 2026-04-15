from app.config import Settings


class PostgresStore:
    def __init__(self, settings: Settings) -> None:
        self.host = settings.snapshot_store_host
        self.port = settings.snapshot_store_port
        self.database = settings.snapshot_store_db

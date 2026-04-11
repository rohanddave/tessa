package domain

const (
	ChunkIndexRequestedEvent  = "chunk.index_requested"
	ChunkDeleteRequestedEvent = "chunk.delete_requested"
)

type ChunkIndexingEvent struct {
	EventType  string `json:"event_type"`
	ChunkID    string `json:"chunk_id"`
	RepoURL    string `json:"repo_url"`
	Branch     string `json:"branch"`
	SnapshotID string `json:"snapshot_id"`
	FileName   string `json:"file_name"`
	FilePath   string `json:"file_path"`
}

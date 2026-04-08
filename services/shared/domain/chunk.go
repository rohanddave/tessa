package domain

type Chunk struct {
	ChunkID      string `json:"chunk_id"`
	RepoURL      string `json:"repo_url"`
	FileHash     string `json:"file_hash"`
	FileName     string `json:"file_name"`
	Branch       string `json:"branch"`
	CommitSHA    string `json:"commit_sha"`
	SnapshotID   string `json:"snapshot_id"`
	FilePath     string `json:"file_path"`
	DocumentType string `json:"document_type"`
	Language     string `json:"language"`
	Text         string `json:"text"`
	SymbolName   string `json:"symbol_name"`
	SymbolType   string `json:"symbol_type"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
	StartChar    int    `json:"start_char"`
	EndChar      int    `json:"end_char"`
	PrevChunkID  string `json:"prev_chunk_id,omitempty"`
	NextChunkID  string `json:"next_chunk_id,omitempty"`
	CreatedAt    int64  `json:"created_at"`
}

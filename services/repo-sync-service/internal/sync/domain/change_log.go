package domain

type ChangeLog struct {
	RepoURL           string                 `json:"repo_url"`
	Branch            string                 `json:"branch"`
	CommitSHA         string                 `json:"commit_sha"`
	PreviousCommitSHA string                 `json:"previous_commit_sha"`
	Created           []ChangeLogFile        `json:"created"`
	Updated           []UpdatedChangeLogFile `json:"updated"`
	Deleted           []ChangeLogFile        `json:"deleted"`
}

type ChangeLogFile struct {
	Path     string `json:"path"`
	FileHash string `json:"file_hash"`
	FileSize int64  `json:"file_size"`
}

type UpdatedChangeLogFile struct {
	Path        string `json:"path"`
	OldFileHash string `json:"old_file_hash"`
	OldFileSize int64  `json:"old_file_size"`
	NewFileHash string `json:"new_file_hash"`
	NewFileSize int64  `json:"new_file_size"`
}

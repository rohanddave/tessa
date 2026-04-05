package domain

type Manifest struct {
	Id        string
	RepoURL   string
	Branch    string
	CommitSHA string
	Files     map[string]ManifestFile
}

type ManifestFile struct {
	FileHash string
	FileSize int64
}

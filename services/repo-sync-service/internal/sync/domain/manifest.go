package domain

type Manifest struct {
	Id        string
	RepoURL   string
	Branch    string
	CommitSHA string
	Files     []ManifestFile
}

type ManifestFile struct {
	FilePath string
	FileHash string
}

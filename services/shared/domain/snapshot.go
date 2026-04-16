package domain

type Snapshot struct {
	Id           string
	RepoURL      string
	Branch       string
	CommitSHA    string
	ManifestURL  string
	ChangeLogURL string
	Status       string
}

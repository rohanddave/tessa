package ports

type StateGateRepo interface {
	SetRepoState(repoURL string, state string) error

	GetRepoState(repoURL string) (string, error)
}

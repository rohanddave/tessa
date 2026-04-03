package ports

type StateGateRepo interface {
	TryStartRegistration(repoURL string) (bool, error)
	MarkRegistered(repoURL string) error
	TryStartDeletion(repoURL string) (bool, error)
	MarkDeleted(repoURL string) error
}

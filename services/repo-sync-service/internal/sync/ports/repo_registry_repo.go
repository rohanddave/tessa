package ports

type RepoRegistryRepo interface {
	TryStartRegistration(repoURL string, branch string, commitSHA string) (bool, error)
	MarkRegistered(repoURL string) error
	TryStartDeletion(repoURL string) (bool, error)
	MarkDeleted(repoURL string) error
	TryUpdateRepo(repoURL string) (bool, error)
}

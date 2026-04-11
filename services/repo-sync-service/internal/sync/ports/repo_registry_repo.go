package ports

import "context"

type RepoRegistryRepo interface {
	TryStartRegistration(ctx context.Context, repoURL string, branch string, commitSHA string) (bool, error)
	MarkRegistered(ctx context.Context, repoURL string) error
	TryStartDeletion(ctx context.Context, repoURL string) (bool, error)
	MarkDeleted(ctx context.Context, repoURL string) error
	TryUpdateRepo(ctx context.Context, repoURL string, branch string) (bool, error)
	MarkUpdated(ctx context.Context, repoURL string, commitSHA string) error
}

package ports

import (
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/domain"
)

type SnapshotStoreRepo interface {
	CreateSnapshot(snapshot *domain.Snapshot) (string, error) // returns the id of the created snapshot

	GetSnapshot(snapshotId string) (*domain.Snapshot, error)

	DeleteSnapshotsByRepoURL(repoURL string) error
}

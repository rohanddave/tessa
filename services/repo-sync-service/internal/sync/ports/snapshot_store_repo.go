package ports

import (
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
)

type SnapshotStoreRepo interface {
	CreateSnapshot(snapshot *shareddomain.Snapshot) (string, error) // returns the id of the created snapshot

	GetSnapshot(snapshotId string) (*shareddomain.Snapshot, error)

	GetLatestSnapshot(repoURL string) (*shareddomain.Snapshot, error)

	DeleteSnapshotsByRepoURL(repoURL string) error
}

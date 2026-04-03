package service

import (
	"fmt"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
)

type DeleteRepoService struct {
	stateGateRepo     ports.StateGateRepo
	snapshotStoreRepo ports.SnapshotStoreRepo
	blobStoreRepo     ports.BlobStoreRepo

	repoURL string
}

func NewDeleteRepoService(repoURL string, snapshotStoreRepo ports.SnapshotStoreRepo, blobStoreRepo ports.BlobStoreRepo, stateGateRepo ports.StateGateRepo) *DeleteRepoService {
	return &DeleteRepoService{
		stateGateRepo:     stateGateRepo,
		snapshotStoreRepo: snapshotStoreRepo,
		blobStoreRepo:     blobStoreRepo,
		repoURL:           repoURL,
	}
}

func (s *DeleteRepoService) DeleteRepo() error {
	started, err := s.stateGateRepo.TryStartDeletion(s.repoURL)
	if err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("repo deletion already in progress or repo is not eligible for deletion: %s", s.repoURL)
	}

	err = s.snapshotStoreRepo.DeleteSnapshotsByRepoURL(s.repoURL)
	if err != nil {
		return err
	}

	err = s.blobStoreRepo.RemoveDirectory(s.repoURL) // assuming that all files for a repo are stored under a directory named after the repo URL
	if err != nil {
		return err
	}

	err = s.stateGateRepo.MarkDeleted(s.repoURL)
	if err != nil {
		return err
	}

	return nil
}

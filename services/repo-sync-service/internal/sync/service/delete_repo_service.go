package service

import (
	"fmt"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type DeleteRepoServiceInput struct {
	RepoURL   string
	Branch    string
	CommitSHA string
}

type DeleteRepoService struct {
	repoRegistryRepo  ports.RepoRegistryRepo
	snapshotStoreRepo ports.SnapshotStoreRepo
	blobStoreRepo     ports.BlobStoreRepo

	repoURL   string
	branch    string
	commitSHA string
}

func NewDeleteRepoService(input *DeleteRepoServiceInput, snapshotStoreRepo ports.SnapshotStoreRepo, blobStoreRepo ports.BlobStoreRepo, repoRegistryRepo ports.RepoRegistryRepo) *DeleteRepoService {
	return &DeleteRepoService{
		repoRegistryRepo:  repoRegistryRepo,
		snapshotStoreRepo: snapshotStoreRepo,
		blobStoreRepo:     blobStoreRepo,
		repoURL:           input.RepoURL,
		branch:            input.Branch,
		commitSHA:         input.CommitSHA,
	}
}

func (s *DeleteRepoService) DeleteRepo() (err error) {
	started, err := s.repoRegistryRepo.TryStartDeletion(s.repoURL)
	if err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("repo deletion already in progress or repo is not eligible for deletion: %s", s.repoURL)
	}

	defer func() {
		if err == nil {
			return
		}

		cleanupErr := s.repoRegistryRepo.MarkRegistered(s.repoURL)
		if cleanupErr != nil {
			err = fmt.Errorf("%w; additionally failed to reset repo registry state: %v", err, cleanupErr)
		}
	}()

	err = s.snapshotStoreRepo.DeleteSnapshotsByRepoURL(s.repoURL)
	if err != nil {
		return err
	}

	err = s.blobStoreRepo.RemoveDirectory(sharedutil.HashString(s.repoURL)) // assuming that all files for a repo are stored under a directory named after the repo URL
	if err != nil {
		return err
	}

	err = s.repoRegistryRepo.MarkDeleted(s.repoURL)
	return err
}

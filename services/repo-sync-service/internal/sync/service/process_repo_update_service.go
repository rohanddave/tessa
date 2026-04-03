package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/domain"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/util"
)

type RepoUpdateServiceInput struct {
	RepoURL   string
	Branch    string
	CommitSHA string
}

type RepoUpdateService struct {
	repoRegistryRepo  ports.RepoRegistryRepo
	dataSourceRepo    ports.DataSourceRepo
	blobStoreRepo     ports.BlobStoreRepo
	snapshotStoreRepo ports.SnapshotStoreRepo

	repoURL   string
	branch    string
	commitSHA string

	blobStorageDestination string

	manifestMu sync.Mutex
	manifest   domain.Manifest
	seenPaths  map[string]struct{}
}

func NewRepoUpdateService(input *RepoUpdateServiceInput, repoRegistryRepo ports.RepoRegistryRepo, dataSourceRepo ports.DataSourceRepo, blobStoreRepo ports.BlobStoreRepo, snapshotStoreRepo ports.SnapshotStoreRepo) *RepoUpdateService {
	return &RepoUpdateService{
		repoRegistryRepo:       repoRegistryRepo,
		dataSourceRepo:         dataSourceRepo,
		blobStoreRepo:          blobStoreRepo,
		snapshotStoreRepo:      snapshotStoreRepo,
		repoURL:                input.RepoURL,
		branch:                 input.Branch,
		commitSHA:              input.CommitSHA,
		blobStorageDestination: util.HashString(input.RepoURL),

		seenPaths: make(map[string]struct{}),
		manifest: domain.Manifest{
			Id:        "",
			RepoURL:   input.RepoURL,
			Branch:    input.Branch,
			CommitSHA: input.CommitSHA,
			Files:     map[string]domain.ManifestFile{},
		},
	}
}

func (s *RepoUpdateService) UpdateRepo() error {
	// check if state is registered for this repo
	updateAllowed, err := s.repoRegistryRepo.TryUpdateRepo(s.repoURL, s.branch)
	if err != nil {
		return fmt.Errorf("try update repo for %q: %w", s.repoURL, err)
	}
	if !updateAllowed {
		return fmt.Errorf("cannot update repo for %q: not in registered state", s.repoURL)
	}

	prevSnapshot, err := s.snapshotStoreRepo.GetLatestSnapshot(s.repoURL)
	if err != nil {
		return fmt.Errorf("get latest snapshot for %q: %w", s.repoURL, err)
	}

	if prevSnapshot == nil {
		return fmt.Errorf("no previous snapshot found for repo %q, cannot perform update", s.repoURL)
	}

	if prevSnapshot.CommitSHA == s.commitSHA {
		// no update needed
		return nil
	}

	prevManifestBytes, err := s.blobStoreRepo.GetFile(prevSnapshot.ManifestURL)
	if err != nil {
		return fmt.Errorf("get manifest from blob store for %q: %w", s.repoURL, err)
	}

	var prevManifest domain.Manifest
	if err := json.Unmarshal(prevManifestBytes, &prevManifest); err != nil {
		return fmt.Errorf("unmarshal previous manifest for %q: %w", s.repoURL, err)
	}

	s.manifest = domain.Manifest{
		Id:        "",
		RepoURL:   s.repoURL,
		Branch:    s.branch,
		CommitSHA: s.commitSHA,
		Files:     make(map[string]domain.ManifestFile, len(prevManifest.Files)),
	}

	for path, file := range prevManifest.Files {
		s.manifest.Files[path] = file
	}

	fileJobs := make(chan FileJob)
	workerErrors := make(chan error, 5)

	var wg sync.WaitGroup

	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.processFileJobsAndAttachToManifest(fileJobs); err != nil {
				workerErrors <- err
			}
		}()
	}

	streamErr := s.streamRepoFiles(fileJobs)
	close(fileJobs)
	wg.Wait()
	close(workerErrors)

	if streamErr != nil {
		return streamErr
	}

	for err := range workerErrors {
		if err != nil {
			return err
		}
	}

	for path := range s.manifest.Files {
		if _, seen := s.seenPaths[path]; !seen {
			delete(s.manifest.Files, path)
		}
	}

	manifestBytes, err := json.Marshal(s.manifest)
	if err != nil {
		return err
	}

	manifestFileName := util.HashString(s.repoURL+s.branch+s.commitSHA) + "_manifest.json"

	// create a manifest file and insert into blob store
	manifestURL, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+manifestFileName, manifestBytes)
	if err != nil {
		return err
	}

	// create snapshot in snapshot store with urls of the manifest and raw files in blob store
	_, err = s.snapshotStoreRepo.CreateSnapshot(&domain.Snapshot{
		RepoURL:     s.repoURL,
		Branch:      s.branch,
		CommitSHA:   s.commitSHA,
		ManifestURL: manifestURL,
	})

	if err != nil {
		return err
	}

	return s.repoRegistryRepo.MarkRegistered(s.repoURL)
}

func (s *RepoUpdateService) streamRepoFiles(jobs chan<- FileJob) error {
	// open repo archive stream from data source
	fileStream, err := s.dataSourceRepo.OpenRepoArchive(ports.OpenRepoFileStreamInput{
		RepoURL: s.repoURL,
		Branch:  s.branch,
		Ref:     s.commitSHA,
	})

	if err != nil {
		return err
	}

	defer fileStream.Close()

	for {
		repoFile, err := fileStream.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}

		content, err := io.ReadAll(repoFile.Reader)
		if err != nil {
			return err
		}

		jobs <- FileJob{
			Path:    repoFile.Path,
			Size:    repoFile.Size,
			Content: content,
		}
	}

	return nil
}

func (s *RepoUpdateService) processFileJobsAndAttachToManifest(jobs <-chan FileJob) error {
	for job := range jobs {
		// insert file content into blob store and get the url
		contentHash := util.HashContent(job.Content)

		s.manifestMu.Lock()
		prevFile, exists := s.manifest.Files[job.Path]
		newFile := domain.ManifestFile{
			FileHash: contentHash,
			FileSize: job.Size,
		}

		if exists && prevFile.FileHash == contentHash {
			// unchanged
			s.manifest.Files[job.Path] = newFile
			s.manifestMu.Unlock()
			continue
		}

		// new or changed
		s.manifest.Files[job.Path] = newFile
		s.manifestMu.Unlock()

		_, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+contentHash, job.Content)
		if err != nil {
			return err
		}
	}
	return nil
}

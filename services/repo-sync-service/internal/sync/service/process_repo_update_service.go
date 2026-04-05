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
	changeLog  domain.ChangeLog
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
		changeLog: domain.ChangeLog{
			RepoURL:   input.RepoURL,
			Branch:    input.Branch,
			CommitSHA: input.CommitSHA,
			Created:   []domain.ChangeLogFile{},
			Updated:   []domain.UpdatedChangeLogFile{},
			Deleted:   []domain.ChangeLogFile{},
		},
	}
}

func (s *RepoUpdateService) UpdateRepo() (err error) {
	// check if state is registered for this repo
	updateAllowed, err := s.repoRegistryRepo.TryUpdateRepo(s.repoURL, s.branch)
	if err != nil {
		return fmt.Errorf("try update repo for %q: %w", s.repoURL, err)
	}
	if !updateAllowed {
		return fmt.Errorf("cannot update repo for %q: not in registered state", s.repoURL)
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

	s.seenPaths = make(map[string]struct{}, len(prevManifest.Files))

	s.manifest = domain.Manifest{
		Id:        "",
		RepoURL:   s.repoURL,
		Branch:    s.branch,
		CommitSHA: s.commitSHA,
		Files:     make(map[string]domain.ManifestFile, len(prevManifest.Files)),
	}
	s.changeLog = domain.ChangeLog{
		RepoURL:           s.repoURL,
		Branch:            s.branch,
		CommitSHA:         s.commitSHA,
		PreviousCommitSHA: prevSnapshot.CommitSHA,
		Created:           []domain.ChangeLogFile{},
		Updated:           []domain.UpdatedChangeLogFile{},
		Deleted:           []domain.ChangeLogFile{},
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
		err = streamErr
		return err
	}

	for workerErr := range workerErrors {
		if workerErr != nil {
			err = workerErr
			return err
		}
	}

	for path, file := range s.manifest.Files {
		if _, seen := s.seenPaths[path]; !seen {
			s.changeLog.Deleted = append(s.changeLog.Deleted, domain.ChangeLogFile{
				Path:     path,
				FileHash: file.FileHash,
				FileSize: file.FileSize,
			})
			delete(s.manifest.Files, path)
		}
	}

	manifestBytes, err := json.Marshal(s.manifest)
	if err != nil {
		return err
	}
	changeLogBytes, err := json.Marshal(s.changeLog)
	if err != nil {
		return err
	}

	manifestFileName := util.HashString(s.repoURL+s.branch+s.commitSHA) + "_manifest.json"
	changeLogFileName := util.HashString(s.repoURL+s.branch+s.commitSHA) + "_change_log.json"

	// create a manifest file and insert into blob store
	manifestURL, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+manifestFileName, manifestBytes)
	if err != nil {
		return err
	}
	changeLogURL, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+changeLogFileName, changeLogBytes)
	if err != nil {
		return err
	}

	// create snapshot in snapshot store with urls of the manifest and raw files in blob store
	_, err = s.snapshotStoreRepo.CreateSnapshot(&domain.Snapshot{
		RepoURL:      s.repoURL,
		Branch:       s.branch,
		CommitSHA:    s.commitSHA,
		ManifestURL:  manifestURL,
		ChangeLogURL: changeLogURL,
	})

	if err != nil {
		return err
	}

	err = s.repoRegistryRepo.MarkUpdated(s.repoURL, s.commitSHA)
	return err
}

func (s *RepoUpdateService) streamRepoFiles(jobs chan<- FileJob) error {
	// open repo archive stream from data source
	fileStream, err := s.dataSourceRepo.OpenRepoArchive(ports.OpenRepoFileStreamInput{
		RepoURL:   s.repoURL,
		Branch:    s.branch,
		CommitSHA: s.commitSHA,
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
		s.seenPaths[job.Path] = struct{}{}
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

		if exists {
			s.changeLog.Updated = append(s.changeLog.Updated, domain.UpdatedChangeLogFile{
				Path:        job.Path,
				OldFileHash: prevFile.FileHash,
				OldFileSize: prevFile.FileSize,
				NewFileHash: newFile.FileHash,
				NewFileSize: newFile.FileSize,
			})
		} else {
			s.changeLog.Created = append(s.changeLog.Created, domain.ChangeLogFile{
				Path:     job.Path,
				FileHash: newFile.FileHash,
				FileSize: newFile.FileSize,
			})
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

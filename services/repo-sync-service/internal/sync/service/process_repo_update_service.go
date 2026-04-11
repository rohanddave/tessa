package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/domain"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
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
	changeLog  shareddomain.ChangeLog
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
		blobStorageDestination: sharedutil.HashString(input.RepoURL),

		seenPaths: make(map[string]struct{}),
		manifest: domain.Manifest{
			Id:        "",
			RepoURL:   input.RepoURL,
			Branch:    input.Branch,
			CommitSHA: input.CommitSHA,
			Files:     map[string]domain.ManifestFile{},
		},
		changeLog: shareddomain.ChangeLog{
			RepoURL:   input.RepoURL,
			Branch:    input.Branch,
			CommitSHA: input.CommitSHA,
			Created:   []shareddomain.ChangeLogFile{},
			Updated:   []shareddomain.UpdatedChangeLogFile{},
			Deleted:   []shareddomain.ChangeLogFile{},
		},
	}
}

func (s *RepoUpdateService) UpdateRepo() (snapshot *shareddomain.Snapshot, err error) {
	// check if state is registered for this repo
	updateAllowed, err := s.repoRegistryRepo.TryUpdateRepo(context.Background(), s.repoURL, s.branch)
	if err != nil {
		return nil, fmt.Errorf("try update repo for %q: %w", s.repoURL, err)
	}
	if !updateAllowed {
		return nil, fmt.Errorf("cannot update repo for %q: not in registered state", s.repoURL)
	}

	defer func() {
		if err == nil {
			return
		}

		cleanupErr := s.repoRegistryRepo.MarkRegistered(context.Background(), s.repoURL)
		if cleanupErr != nil {
			err = fmt.Errorf("%w; additionally failed to reset repo registry state: %v", err, cleanupErr)
		}
	}()

	prevSnapshot, err := s.snapshotStoreRepo.GetLatestSnapshot(s.repoURL)
	if err != nil {
		return nil, fmt.Errorf("get latest snapshot for %q: %w", s.repoURL, err)
	}

	if prevSnapshot == nil {
		return nil, fmt.Errorf("no previous snapshot found for repo %q, cannot perform update", s.repoURL)
	}

	if prevSnapshot.CommitSHA == s.commitSHA {
		// no update needed
		return nil, nil
	}

	prevManifestFile, err := s.blobStoreRepo.GetFile(prevSnapshot.ManifestURL)
	if err != nil {
		return nil, fmt.Errorf("get manifest from blob store for %q: %w", s.repoURL, err)
	}

	var prevManifest domain.Manifest
	if err := json.Unmarshal(prevManifestFile.Content, &prevManifest); err != nil {
		return nil, fmt.Errorf("unmarshal previous manifest for %q: %w", s.repoURL, err)
	}

	s.seenPaths = make(map[string]struct{}, len(prevManifest.Files))

	s.manifest = domain.Manifest{
		Id:        "",
		RepoURL:   s.repoURL,
		Branch:    s.branch,
		CommitSHA: s.commitSHA,
		Files:     make(map[string]domain.ManifestFile, len(prevManifest.Files)),
	}
	s.changeLog = shareddomain.ChangeLog{
		RepoURL:           s.repoURL,
		Branch:            s.branch,
		CommitSHA:         s.commitSHA,
		PreviousCommitSHA: prevSnapshot.CommitSHA,
		Created:           []shareddomain.ChangeLogFile{},
		Updated:           []shareddomain.UpdatedChangeLogFile{},
		Deleted:           []shareddomain.ChangeLogFile{},
	}

	for path, file := range prevManifest.Files {
		s.manifest.Files[path] = file
	}

	fileJobs := make(chan shareddomain.FileJob)
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
		return nil, err
	}

	for workerErr := range workerErrors {
		if workerErr != nil {
			err = workerErr
			return nil, err
		}
	}

	for path, file := range s.manifest.Files {
		if _, seen := s.seenPaths[path]; !seen {
			s.changeLog.Deleted = append(s.changeLog.Deleted, shareddomain.ChangeLogFile{
				Path:          path,
				FileHash:      file.FileHash,
				FileSize:      file.FileSize,
				FileExtension: file.FileExtension,
			})
			delete(s.manifest.Files, path)
		}
	}

	manifestBytes, err := json.Marshal(s.manifest)
	if err != nil {
		return nil, err
	}
	changeLogBytes, err := json.Marshal(s.changeLog)
	if err != nil {
		return nil, err
	}

	manifestFileName := sharedutil.HashString(s.repoURL+s.branch+s.commitSHA) + "_manifest.json"
	changeLogFileName := sharedutil.HashString(s.repoURL+s.branch+s.commitSHA) + "_change_log.json"

	// create a manifest file and insert into blob store
	manifestURL, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+manifestFileName, manifestBytes, "json")
	if err != nil {
		return nil, err
	}
	changeLogURL, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+changeLogFileName, changeLogBytes, "json")
	if err != nil {
		return nil, err
	}

	// create snapshot in snapshot store with urls of the manifest and raw files in blob store
	snapshot = &shareddomain.Snapshot{
		RepoURL:      s.repoURL,
		Branch:       s.branch,
		CommitSHA:    s.commitSHA,
		ManifestURL:  manifestURL,
		ChangeLogURL: changeLogURL,
	}

	snapshotId, err := s.snapshotStoreRepo.CreateSnapshot(snapshot)

	if err != nil {
		return nil, err
	}

	snapshot.Id = snapshotId

	// mark repo registry state as updated
	err = s.repoRegistryRepo.MarkUpdated(context.Background(), s.repoURL, s.commitSHA)

	return snapshot, err
}

func (s *RepoUpdateService) streamRepoFiles(jobs chan<- shareddomain.FileJob) error {
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

		jobs <- shareddomain.FileJob{
			Path:      repoFile.Path,
			Size:      repoFile.Size,
			Content:   content,
			Extension: repoFile.Extension,
		}
	}

	return nil
}

func (s *RepoUpdateService) processFileJobsAndAttachToManifest(jobs <-chan shareddomain.FileJob) error {
	for job := range jobs {
		// insert file content into blob store and get the url
		contentHash := sharedutil.HashContent(job.Content)

		s.manifestMu.Lock()
		s.seenPaths[job.Path] = struct{}{}
		prevFile, exists := s.manifest.Files[job.Path]
		newFile := domain.ManifestFile{
			FileHash:      contentHash,
			FileSize:      job.Size,
			FileExtension: job.Extension,
		}

		if exists && prevFile.FileHash == contentHash {
			// unchanged
			s.manifest.Files[job.Path] = newFile
			s.manifestMu.Unlock()
			continue
		}

		if exists {
			s.changeLog.Updated = append(s.changeLog.Updated, shareddomain.UpdatedChangeLogFile{
				Path:          job.Path,
				OldFileHash:   prevFile.FileHash,
				OldFileSize:   prevFile.FileSize,
				NewFileHash:   newFile.FileHash,
				NewFileSize:   newFile.FileSize,
				FileExtension: newFile.FileExtension,
			})
		} else {
			s.changeLog.Created = append(s.changeLog.Created, shareddomain.ChangeLogFile{
				Path:          job.Path,
				FileHash:      newFile.FileHash,
				FileSize:      newFile.FileSize,
				FileExtension: newFile.FileExtension,
			})
		}

		// new or changed
		s.manifest.Files[job.Path] = newFile
		s.manifestMu.Unlock()

		_, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+contentHash, job.Content, job.Extension)
		if err != nil {
			return err
		}
	}
	return nil
}

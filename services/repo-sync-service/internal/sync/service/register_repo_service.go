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
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
)

type RegisterRepoServiceInput struct {
	RepoURL   string
	Branch    string
	CommitSHA string
}

type RegisterRepoService struct {
	repoRegistryRepo  ports.RepoRegistryRepo
	dataSourceRepo    ports.DataSourceRepo
	snapshotStoreRepo ports.SnapshotStoreRepo
	blobStoreRepo     ports.BlobStoreRepo

	repoURL   string
	branch    string
	commitSHA string

	blobStorageDestination string

	manifestMu sync.Mutex
	manifest   domain.Manifest
	changeLog  shareddomain.ChangeLog
}

func NewRegisterRepoService(input *RegisterRepoServiceInput, dataSourceRepo ports.DataSourceRepo, snapshotStoreRepo ports.SnapshotStoreRepo, blobStoreRepo ports.BlobStoreRepo, repoRegistryRepo ports.RepoRegistryRepo) *RegisterRepoService {
	return &RegisterRepoService{
		repoRegistryRepo:       repoRegistryRepo,
		dataSourceRepo:         dataSourceRepo,
		snapshotStoreRepo:      snapshotStoreRepo,
		blobStoreRepo:          blobStoreRepo,
		repoURL:                input.RepoURL,
		branch:                 input.Branch,
		commitSHA:              input.CommitSHA,
		blobStorageDestination: util.HashString(input.RepoURL),
		manifest: domain.Manifest{
			Id:        "",
			RepoURL:   input.RepoURL,
			Branch:    input.Branch,
			CommitSHA: input.CommitSHA,
			Files:     map[string]domain.ManifestFile{},
		},
		changeLog: shareddomain.ChangeLog{
			RepoURL:           input.RepoURL,
			Branch:            input.Branch,
			CommitSHA:         input.CommitSHA,
			PreviousCommitSHA: "",
			Created:           []shareddomain.ChangeLogFile{},
			Updated:           []shareddomain.UpdatedChangeLogFile{},
			Deleted:           []shareddomain.ChangeLogFile{},
		},
	}
}

func (s *RegisterRepoService) RegisterRepo() (snapshot *shareddomain.Snapshot, err error) {
	started, err := s.repoRegistryRepo.TryStartRegistration(s.repoURL, s.branch, s.commitSHA)
	if err != nil {
		return nil, err
	}
	if !started {
		return nil, fmt.Errorf("repo registration already in progress or repo is not eligible for registration: %s", s.repoURL)
	}

	defer func() {
		if err == nil {
			return
		}

		cleanupErr := s.repoRegistryRepo.MarkDeleted(s.repoURL)
		if cleanupErr != nil {
			err = fmt.Errorf("%w; additionally failed to reset repo registry state: %v", err, cleanupErr)
		}
	}()

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

	manifestBytes, err := json.Marshal(s.manifest)
	if err != nil {
		return nil, err
	}
	changeLogBytes, err := json.Marshal(s.buildInitialChangeLog())
	if err != nil {
		return nil, err
	}

	manifestFileName := util.HashString(s.repoURL+s.branch+s.commitSHA) + "_manifest.json"
	changeLogFileName := util.HashString(s.repoURL+s.branch+s.commitSHA) + "_change_log.json"

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

	// mark repo registry state as registered
	err = s.repoRegistryRepo.MarkRegistered(s.repoURL)

	return snapshot, err
}

func (s *RegisterRepoService) streamRepoFiles(jobs chan<- shareddomain.FileJob) error {
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

func (s *RegisterRepoService) processFileJobsAndAttachToManifest(jobs <-chan shareddomain.FileJob) error {
	for job := range jobs {
		// insert file content into blob store and get the url
		contentHash := util.HashContent(job.Content)
		_, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+contentHash, job.Content, job.Extension)
		if err != nil {
			return err
		}

		s.manifestMu.Lock()
		s.manifest.Files[job.Path] = domain.ManifestFile{
			FileHash:      contentHash,
			FileSize:      job.Size,
			FileExtension: job.Extension,
		}
		s.manifestMu.Unlock()
	}
	return nil
}

func (s *RegisterRepoService) buildInitialChangeLog() shareddomain.ChangeLog {
	s.manifestMu.Lock()
	defer s.manifestMu.Unlock()

	created := make([]shareddomain.ChangeLogFile, 0, len(s.manifest.Files))
	for path, file := range s.manifest.Files {
		created = append(created, shareddomain.ChangeLogFile{
			Path:          path,
			FileHash:      file.FileHash,
			FileSize:      file.FileSize,
			FileExtension: file.FileExtension,
		})
	}

	s.changeLog.Created = created
	return s.changeLog
}

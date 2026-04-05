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

type FileJob struct {
	Path    string
	Size    int64
	Content []byte
}

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
	}
}

func (s *RegisterRepoService) RegisterRepo() (err error) {
	started, err := s.repoRegistryRepo.TryStartRegistration(s.repoURL, s.branch, s.commitSHA)
	if err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("repo registration already in progress or repo is not eligible for registration: %s", s.repoURL)
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
		RepoURL:      s.repoURL,
		Branch:       s.branch,
		CommitSHA:    s.commitSHA,
		ManifestURL:  manifestURL,
		ChangeLogURL: "",
	})

	if err != nil {
		return err
	}

	err = s.repoRegistryRepo.MarkRegistered(s.repoURL)
	return err
}

func (s *RegisterRepoService) streamRepoFiles(jobs chan<- FileJob) error {
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

func (s *RegisterRepoService) processFileJobsAndAttachToManifest(jobs <-chan FileJob) error {
	for job := range jobs {
		// insert file content into blob store and get the url
		contentHash := util.HashContent(job.Content)
		_, err := s.blobStoreRepo.InsertFile(s.blobStorageDestination+"/"+contentHash, job.Content)
		if err != nil {
			return err
		}

		s.manifestMu.Lock()
		s.manifest.Files[job.Path] = domain.ManifestFile{
			FileHash: contentHash,
			FileSize: job.Size,
		}
		s.manifestMu.Unlock()
	}
	return nil
}

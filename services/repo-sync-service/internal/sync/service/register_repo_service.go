package service

import (
	"encoding/json"
	"errors"
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
	stateGateRepo     ports.StateGateRepo
	dataSourceRepo    ports.DataSourceRepo
	snapshotStoreRepo ports.SnapshotStoreRepo
	blobStoreRepo     ports.BlobStoreRepo

	repoURL   string
	branch    string
	commitSHA string

	manifestMu sync.Mutex
	manifest   domain.Manifest
}

func NewRegisterRepoService(input RegisterRepoServiceInput, dataSourceRepo ports.DataSourceRepo, snapshotStoreRepo ports.SnapshotStoreRepo, blobStoreRepo ports.BlobStoreRepo, stateGateRepo ports.StateGateRepo) *RegisterRepoService {
	return &RegisterRepoService{
		stateGateRepo:     stateGateRepo,
		dataSourceRepo:    dataSourceRepo,
		snapshotStoreRepo: snapshotStoreRepo,
		blobStoreRepo:     blobStoreRepo,
		repoURL:           input.RepoURL,
		branch:            input.Branch,
		commitSHA:         input.CommitSHA,
		manifest: domain.Manifest{
			Id:        "",
			RepoURL:   input.RepoURL,
			Branch:    input.Branch,
			CommitSHA: input.CommitSHA,
			Files:     []domain.ManifestFile{},
		},
	}
}

func (s *RegisterRepoService) RegisterRepo() error {
	// this function itself should use a mutex lock to ensure that only one registration process can happen for a repo at a time, and it should also check the state gate repo to see if the repo is already being registered or is already registered before proceeding with the registration
	err := s.stateGateRepo.SetRepoState(s.repoURL, "registering")
	if err != nil {
		return err
	}

	fileJobs := make(chan FileJob)
	workerErrors := make(chan error, 5)

	var wg sync.WaitGroup

	for i := 0; i < 5; i++ {
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

	s.manifest.CommitSHA = s.commitSHA
	manifestBytes, err := json.Marshal(s.manifest)
	if err != nil {
		return err
	}

	// create a manifest file and insert into blob store
	manifestURL, err := s.blobStoreRepo.InsertFile(s.repoURL+"/manifest.json", manifestBytes)
	if err != nil {
		return err
	}

	// create snapshot in snapshot store with urls of the manifest and raw files in blob store
	snapshot := domain.Snapshot{
		RepoURL:     s.repoURL,
		Branch:      s.branch,
		CommitSHA:   s.commitSHA,
		ManifestURL: manifestURL,
	}

	_, err = s.snapshotStoreRepo.CreateSnapshot(snapshot)
	if err != nil {
		return err
	}

	return s.stateGateRepo.SetRepoState(s.repoURL, "registered")
}

func (s *RegisterRepoService) streamRepoFiles(jobs chan<- FileJob) error {
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

	if fileStream.CommitSHA() != "" {
		s.commitSHA = fileStream.CommitSHA()
	}

	return nil
}

func (s *RegisterRepoService) processFileJobsAndAttachToManifest(jobs <-chan FileJob) error {
	for job := range jobs {
		// insert file content into blob store and get the url
		contentHash := util.HashContent(job.Content)
		_, err := s.blobStoreRepo.InsertFile(s.repoURL+"/"+contentHash, job.Content)
		if err != nil {
			return err
		}

		s.manifestMu.Lock()
		s.manifest.Files = append(s.manifest.Files, domain.ManifestFile{
			FilePath: job.Path,
			FileHash: contentHash,
		})
		s.manifestMu.Unlock()
	}
	return nil
}

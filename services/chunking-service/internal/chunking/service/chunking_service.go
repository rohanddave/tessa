package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path"
	"sync"

	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
)

type ChunkingServiceInput struct {
	Logger               *log.Logger
	BlobStoreRepo        *sharedblobstore.Repo
	NormalizationService *NormalizationService
	ExtractionService    *ExtractionService
}

type ChunkingService struct {
	logger               *log.Logger
	normalizationService *NormalizationService
	extractionService    *ExtractionService
	blobStoreRepo        *sharedblobstore.Repo
}

func NewChunkingService(input *ChunkingServiceInput) *ChunkingService {
	return &ChunkingService{
		logger:               input.Logger,
		blobStoreRepo:        input.BlobStoreRepo,
		normalizationService: input.NormalizationService,
		extractionService:    input.ExtractionService,
	}
}

func (s *ChunkingService) Start(snapshot shareddomain.Snapshot) (map[string]string, error) {
	s.logf("starting chunking pipeline for snapshot=%s repo=%s changelog=%s", snapshot.Id, snapshot.RepoURL, snapshot.ChangeLogURL)

	changeLogFileContentRaw, err := s.blobStoreRepo.GetFile(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to fetch change log for snapshot=%s: %v", snapshot.Id, err)
		return nil, err
	}
	s.logf("loaded change log for snapshot=%s bytes=%d", snapshot.Id, len(changeLogFileContentRaw))

	var changeLogFileContent shareddomain.ChangeLog
	err = json.Unmarshal(changeLogFileContentRaw, &changeLogFileContent)
	if err != nil {
		s.logf("failed to decode change log for snapshot=%s: %v", snapshot.Id, err)
		return nil, err
	}
	s.logf(
		"decoded change log for snapshot=%s created=%d updated=%d deleted=%d",
		snapshot.Id,
		len(changeLogFileContent.Created),
		len(changeLogFileContent.Updated),
		len(changeLogFileContent.Deleted),
	)

	fetchedFilesChannel := make(chan shareddomain.FileJob)
	normalizedFilesChannel := make(chan string)
	errCh := make(chan error, 1)

	normalizationWorkerCount := 5
	extractionWorkerCount := 5
	fileFetchWorkerCount := 5

	var normalizationWG sync.WaitGroup
	var extractionWG sync.WaitGroup

	for range normalizationWorkerCount {
		normalizationWG.Add(1)
		go s.normalizationWorker(fetchedFilesChannel, normalizedFilesChannel, &normalizationWG, errCh)
	}

	for range extractionWorkerCount {
		extractionWG.Add(1)
		go s.extractionWorker(normalizedFilesChannel, &extractionWG, errCh)
	}

	// TODO: handle updated and deleted files as well
	fileURLs := make([]string, 0, len(changeLogFileContent.Created))
	blobDirectoryURL, err := deriveDirectoryURL(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to derive blob directory from change log url for snapshot=%s: %v", snapshot.Id, err)
		return nil, err
	}

	for _, file := range changeLogFileContent.Created {
		fileURL := blobDirectoryURL + "/" + file.FileHash
		s.logf("queued file for fetching snapshot=%s path=%s hash=%s url=%s size=%d", snapshot.Id, file.Path, file.FileHash, fileURL, file.FileSize)
		fileURLs = append(fileURLs, fileURL)
	}
	s.logf("fetching %d created files for snapshot=%s with %d blob workers", len(fileURLs), snapshot.Id, fileFetchWorkerCount)

	if err := s.blobStoreRepo.GetFiles(fileURLs, fetchedFilesChannel, fileFetchWorkerCount); err != nil {
		s.logf("failed while fetching files for snapshot=%s: %v", snapshot.Id, err)
		close(fetchedFilesChannel)
		close(normalizedFilesChannel)
		return nil, err
	}

	s.logf("completed blob fetch stage for snapshot=%s", snapshot.Id)
	close(fetchedFilesChannel)
	normalizationWG.Wait()
	s.logf("completed normalization stage for snapshot=%s", snapshot.Id)
	close(normalizedFilesChannel)
	extractionWG.Wait()
	s.logf("completed extraction stage for snapshot=%s", snapshot.Id)

	select {
	case workerErr := <-errCh:
		if workerErr != nil {
			s.logf("worker error for snapshot=%s: %v", snapshot.Id, workerErr)
			return nil, workerErr
		}
	default:
	}

	s.logf("finished chunking pipeline for snapshot=%s", snapshot.Id)
	return map[string]string{}, nil
}

func (s *ChunkingService) normalizationWorker(fetchedFilesChannel <-chan shareddomain.FileJob, normalizedFilesChannel chan<- string, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	for fileJob := range fetchedFilesChannel {
		s.logf("normalizing file path=%s size=%d", fileJob.Path, fileJob.Size)
		normalizedContent, err := s.normalizationService.NormalizeFileContent(string(fileJob.Content))
		if err != nil {
			s.logf("normalization failed for file path=%s: %v", fileJob.Path, err)
			select {
			case errCh <- fmt.Errorf("normalize file %s: %w", fileJob.Path, err):
			default:
			}
			continue
		}

		s.logf("normalized file path=%s output_bytes=%d", fileJob.Path, len(normalizedContent))
		normalizedFilesChannel <- normalizedContent
	}
}

func (s *ChunkingService) extractionWorker(normalizedFilesChannel <-chan string, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	for normalizedContent := range normalizedFilesChannel {
		s.logf("extracting normalized content bytes=%d", len(normalizedContent))
		extractedData, err := s.extractionService.ExtractFileContent(normalizedContent)
		if err != nil {
			s.logf("extraction failed: %v", err)
			select {
			case errCh <- fmt.Errorf("extract content: %w", err):
			default:
			}
			continue
		}

		s.logf("extracted data keys=%d", len(extractedData))
	}
}

func (s *ChunkingService) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

func deriveDirectoryURL(fileURL string) (string, error) {
	parsedURL, err := url.Parse(fileURL)
	if err != nil {
		return "", fmt.Errorf("parse file url %q: %w", fileURL, err)
	}

	dirPath := path.Dir(parsedURL.Path)
	if dirPath == "." || dirPath == "/" {
		return "", fmt.Errorf("file url %q does not contain a directory path", fileURL)
	}

	parsedURL.Path = dirPath
	return parsedURL.String(), nil
}

// func (s *ChunkingService) Start(snapshot shareddomain.Snapshot) (map[string]string, error) {
// 	changeLogUrls := snapshot.ChangeLogURL

// 	changeLogFile, err := s.

// 	for _, filePath := range changeLogUrls {
// 		// For each file path, you can read the file content from the blob store or any other source
// 		// This is a placeholder for reading the file content
// 		fileContent := "" // Replace this with actual file content retrieval logic

// 		// Process the file content through normalization, extraction, and chunking
// 		chunkedData, err := s.processFileContent(fileContent, filePath)
// 		if err != nil {
// 			return nil, err
// 		}

// 		// You can store or return the chunked data as needed
// 		_ = chunkedData // Replace this with your actual handling of chunked data
// 	}

// 	return nil, nil // Replace this with your actual return value as needed
// }

// func (s *ChunkingService) processFileContent(fileContent string, filePath string) (map[string]string, error) {

// 	// Step 1: Normalize the file content
// 	normalizedContent, err := s.normalizationService.NormalizeFileContent(fileContent)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Step 2: Extract relevant information from the normalized content
// 	extractedData, err := s.extractionService.ExtractFileContent(normalizedContent, filePath)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Step 3: Chunk the extracted data as needed (this is a placeholder for your actual chunking logic)
// 	chunkedData := make(map[string]string)
// 	for key, value := range extractedData {
// 		chunkedData[key] = value // Replace this with your actual chunking logic
// 	}

// 	return chunkedData, nil
// }

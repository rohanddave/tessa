package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
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

	changeLogFile, err := s.blobStoreRepo.GetFile(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to fetch change log for snapshot=%s: %v", snapshot.Id, err)
		return nil, err
	}
	s.logf("loaded change log for snapshot=%s bytes=%d extension=%s", snapshot.Id, len(changeLogFile.Content), changeLogFile.Extension)

	var changeLogFileContent shareddomain.ChangeLog
	err = json.Unmarshal(changeLogFile.Content, &changeLogFileContent)
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

	fetchedFilesChannel := make(chan *shareddomain.FileJob)
	normalizedFilesChannel := make(chan *shareddomain.FileJob)
	errCh := make(chan error, 1)
	chunkLocations := make(map[string]string)

	blobDirectoryURL, err := deriveDirectoryURL(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to derive blob directory from change log url for snapshot=%s: %v", snapshot.Id, err)
		return nil, err
	}

	normalizationWorkerCount := 5
	extractionWorkerCount := 5
	fileFetchWorkerCount := 5

	var normalizationWG sync.WaitGroup
	var extractionWG sync.WaitGroup
	var chunkLocationsMu sync.Mutex
	var chunkCounter atomic.Uint64

	for range normalizationWorkerCount {
		normalizationWG.Add(1)
		go s.normalizationWorker(fetchedFilesChannel, normalizedFilesChannel, &normalizationWG, errCh)
	}

	for range extractionWorkerCount {
		extractionWG.Add(1)
		go s.extractionWorker(
			normalizedFilesChannel,
			snapshot,
			blobDirectoryURL,
			&chunkCounter,
			chunkLocations,
			&chunkLocationsMu,
			&extractionWG,
			errCh,
		)
	}

	// TODO: handle updated and deleted files as well
	fileURLs := make([]string, 0, len(changeLogFileContent.Created))

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
	return chunkLocations, nil
}

func (s *ChunkingService) normalizationWorker(fetchedFilesChannel <-chan *shareddomain.FileJob, normalizedFilesChannel chan<- *shareddomain.FileJob, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	for fileJob := range fetchedFilesChannel {
		if fileJob == nil {
			s.logf("skipping nil file job in normalization worker")
			continue
		}

		s.logf("normalizing file path=%s size=%d", fileJob.Path, fileJob.Size)
		normalizedFileJob, err := s.normalizationService.NormalizeFileContent(fileJob)
		if err != nil {
			s.logf("normalization failed for file path=%s: %v", fileJob.Path, err)
			select {
			case errCh <- fmt.Errorf("normalize file %s: %w", fileJob.Path, err):
			default:
			}
			continue
		}

		s.logf("normalized file path=%s output_bytes=%d", normalizedFileJob.Path, len(normalizedFileJob.Content))
		normalizedFilesChannel <- normalizedFileJob
	}
}

func (s *ChunkingService) extractionWorker(normalizedFilesChannel <-chan *shareddomain.FileJob, snapshot shareddomain.Snapshot, blobDirectoryURL string, chunkCounter *atomic.Uint64, chunkLocations map[string]string, chunkLocationsMu *sync.Mutex, wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()

	for fileJob := range normalizedFilesChannel {
		if fileJob == nil {
			s.logf("skipping nil file job in extraction worker")
			continue
		}

		s.logf("extracting normalized content path=%s bytes=%d extension=%s", fileJob.Path, len(fileJob.Content), fileJob.Extension)
		extractedData, err := s.extractionService.ExtractFileContent(fileJob)
		if err != nil {
			s.logf("extraction failed path=%s: %v", fileJob.Path, err)
			select {
			case errCh <- fmt.Errorf("extract content for %s: %w", fileJob.Path, err):
			default:
			}
			continue
		}

		s.logf("extraced data symbols=%d containers=%d imports=%d doc comments=%d classes=%d functions=%d methods=%d module level declarations=%d", len(extractedData.Symbols), len(extractedData.Containers), len(extractedData.Imports), len(extractedData.DocComments), len(extractedData.Classes), len(extractedData.Functions), len(extractedData.Methods), len(extractedData.ModuleLevelDeclarations))

		chunkID := strconv.FormatUint(chunkCounter.Add(1), 10)
		chunk := buildChunk(snapshot, fileJob, chunkID, extractedData)

		chunkJSON, err := json.Marshal(chunk)
		if err != nil {
			s.logf("failed to marshal chunk payload: %v", err)
			select {
			case errCh <- fmt.Errorf("marshal chunk payload: %w", err):
			default:
			}
			continue
		}

		contentHash := hashContent(chunkJSON)
		fileName := fmt.Sprintf("%s_chunk_%s.json", contentHash, chunkID)
		filePath := sharedutil.HashString(snapshot.RepoURL) + "/" + fileName

		insertedURL, err := s.blobStoreRepo.InsertFile(filePath, chunkJSON, "json")
		if err != nil {
			s.logf("failed to write chunk chunk_id=%s path=%s: %v", chunkID, filePath, err)
			select {
			case errCh <- fmt.Errorf("insert chunk %s: %w", chunkID, err):
			default:
			}
			continue
		}

		chunkLocationsMu.Lock()
		chunkLocations[chunkID] = insertedURL
		chunkLocationsMu.Unlock()
		s.logf("stored extracted chunk chunk_id=%s url=%s", chunkID, insertedURL)
	}
}

func buildChunk(snapshot shareddomain.Snapshot, fileJob *shareddomain.FileJob, chunkID string, extractedData *ports.CodeParseResult) shareddomain.Chunk {
	filePath := ""
	fileName := ""
	language := "unknown"
	text := ""

	if fileJob != nil {
		filePath = fileJob.Path
		fileName = path.Base(fileJob.Path)
		language = normalizeLanguage(fileJob.Extension)
		text = string(fileJob.Content)
	}

	chunk := shareddomain.Chunk{
		ChunkID:      chunkID,
		RepoURL:      snapshot.RepoURL,
		FileHash:     hashContent([]byte(text)),
		FileName:     fileName,
		Branch:       snapshot.Branch,
		CommitSHA:    snapshot.CommitSHA,
		SnapshotID:   snapshot.Id,
		FilePath:     filePath,
		DocumentType: "code",
		Language:     language,
		Text:         text,
		SymbolName:   "",
		SymbolType:   "",
		StartLine:    0,
		EndLine:      0,
		StartChar:    0,
		EndChar:      0,
		PrevChunkID:  "",
		NextChunkID:  "",
		CreatedAt:    time.Now().UTC().Unix(),
	}

	if extractedData == nil || len(extractedData.Symbols) == 0 {
		return chunk
	}

	primarySymbol := extractedData.Symbols[0]
	chunk.SymbolName = primarySymbol.Name
	chunk.SymbolType = primarySymbol.Kind
	chunk.StartChar = int(primarySymbol.StartByte)
	chunk.EndChar = int(primarySymbol.EndByte)

	return chunk
}

func normalizeLanguage(extension string) string {
	switch strings.ToLower(strings.TrimPrefix(extension, ".")) {
	case "go":
		return "go"
	case "js":
		return "javascript"
	case "jsx":
		return "javascript"
	case "ts":
		return "typescript"
	case "tsx":
		return "typescript"
	case "py":
		return "python"
	case "java":
		return "java"
	default:
		return "unknown"
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

func hashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
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

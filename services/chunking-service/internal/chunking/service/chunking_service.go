package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type ChunkingServiceInput struct {
	Logger               *log.Logger
	BlobStoreRepo        *sharedblobstore.Repo
	ChunkRepo            *ports.ChunkRepo
	NormalizationService *NormalizationService
	ExtractionService    *ExtractionService
}

type ChunkingService struct {
	logger               *log.Logger
	normalizationService *NormalizationService
	extractionService    *ExtractionService
	chunkRepo            *ports.ChunkRepo
	blobStoreRepo        *sharedblobstore.Repo
}

func NewChunkingService(input *ChunkingServiceInput) *ChunkingService {
	return &ChunkingService{
		logger:               input.Logger,
		blobStoreRepo:        input.BlobStoreRepo,
		normalizationService: input.NormalizationService,
		chunkRepo:            input.ChunkRepo,
		extractionService:    input.ExtractionService,
	}
}

func (s *ChunkingService) Start(snapshot shareddomain.Snapshot) error {
	s.logf("starting chunking pipeline for snapshot=%s repo=%s changelog=%s", snapshot.Id, snapshot.RepoURL, snapshot.ChangeLogURL)

	changeLogFile, err := s.blobStoreRepo.GetFile(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to fetch change log for snapshot=%s: %v", snapshot.Id, err)
		return err
	}
	s.logf("loaded change log for snapshot=%s bytes=%d extension=%s", snapshot.Id, len(changeLogFile.Content), changeLogFile.Extension)

	var changeLogFileContent shareddomain.ChangeLog
	err = json.Unmarshal(changeLogFile.Content, &changeLogFileContent)
	if err != nil {
		s.logf("failed to decode change log for snapshot=%s: %v", snapshot.Id, err)
		return err
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

	blobDirectoryURL, err := deriveDirectoryURL(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to derive blob directory from change log url for snapshot=%s: %v", snapshot.Id, err)
		return err
	}

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
		go s.extractionWorker(
			normalizedFilesChannel,
			snapshot,
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
		return err
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
			return workerErr
		}
	default:
	}

	s.logf("finished chunking pipeline for snapshot=%s", snapshot.Id)
	return nil
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

func (s *ChunkingService) extractionWorker(normalizedFilesChannel <-chan *shareddomain.FileJob, snapshot shareddomain.Snapshot, wg *sync.WaitGroup, errCh chan<- error) {
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

		chunks := buildChunks(snapshot, fileJob, extractedData)
		s.logf("built %d chunks for snapshot with id=%s", len(chunks), snapshot.Id)

		for _, chunk := range chunks {
			err = s.chunkRepo.CreateChunk(context.Background(), &chunk)
			if err != nil {
				s.logf("failed to write chunk with id=%s to database: %v", chunk.ChunkID, err)

			}

			s.logf("stored extracted chunk chunk_id=%s symbol=%s type=%s", chunk.ChunkID, chunk.SymbolName, chunk.SymbolType)
		}
	}
}

func buildChunks(snapshot shareddomain.Snapshot, fileJob *shareddomain.FileJob, extractedData *ports.CodeParseResult) []shareddomain.Chunk {
	declarations := collectChunkDeclarations(extractedData)
	if len(declarations) == 0 {
		chunkID := sharedutil.GenerateUUID()
		return []shareddomain.Chunk{buildChunk(snapshot, fileJob, chunkID, ports.CodeDeclaration{
			Name:      "",
			Kind:      "",
			Text:      "",
			StartByte: 0,
			EndByte:   uint32(len(fileJob.Content)),
		})}
	}

	chunks := make([]shareddomain.Chunk, 0, len(declarations))
	for _, declaration := range declarations {
		chunkID := sharedutil.GenerateUUID()
		chunks = append(chunks, buildChunk(snapshot, fileJob, chunkID, declaration))
	}

	for i := range chunks {
		if i > 0 {
			chunks[i].PrevChunkID = chunks[i-1].ChunkID
		}
		if i < len(chunks)-1 {
			chunks[i].NextChunkID = chunks[i+1].ChunkID
		}
	}

	return chunks
}

func collectChunkDeclarations(extractedData *ports.CodeParseResult) []ports.CodeDeclaration {
	if extractedData == nil {
		return nil
	}

	declarations := make([]ports.CodeDeclaration, 0, len(extractedData.Classes)+len(extractedData.Functions)+len(extractedData.Methods)+len(extractedData.ModuleLevelDeclarations))
	seen := make(map[string]struct{})

	appendDeclarations := func(items []ports.CodeDeclaration) {
		for _, declaration := range items {
			if declaration.StartByte >= declaration.EndByte {
				continue
			}

			key := fmt.Sprintf("%s:%s:%d:%d", declaration.Kind, declaration.Name, declaration.StartByte, declaration.EndByte)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			declarations = append(declarations, declaration)
		}
	}

	appendDeclarations(extractedData.Classes)
	appendDeclarations(extractedData.Functions)
	appendDeclarations(extractedData.Methods)
	appendDeclarations(extractedData.ModuleLevelDeclarations)

	return declarations
}

func buildChunk(snapshot shareddomain.Snapshot, fileJob *shareddomain.FileJob, chunkID string, declaration ports.CodeDeclaration) shareddomain.Chunk {
	filePath := ""
	fileName := ""
	language := "unknown"
	text := ""
	startByte := declaration.StartByte
	endByte := declaration.EndByte
	startLine := 0
	endLine := 0

	if fileJob != nil {
		filePath = fileJob.Path
		fileName = path.Base(fileJob.Path)
		language = normalizeLanguage(fileJob.Extension)
		text = extractChunkText(fileJob.Content, startByte, endByte)
		startLine, endLine = byteRangeToLineRange(fileJob.Content, startByte, endByte)
	}

	chunk := shareddomain.Chunk{
		ChunkID:      chunkID,
		RepoURL:      snapshot.RepoURL,
		FileHash:     sharedutil.HashString(text),
		FileName:     fileName,
		Branch:       snapshot.Branch,
		CommitSHA:    snapshot.CommitSHA,
		SnapshotID:   snapshot.Id,
		FilePath:     filePath,
		DocumentType: "code",
		Language:     language,
		Text:         text,
		SymbolName:   declaration.Name,
		SymbolType:   declaration.Kind,
		StartLine:    startLine,
		EndLine:      endLine,
		StartChar:    int(startByte),
		EndChar:      int(endByte),
		PrevChunkID:  "",
		NextChunkID:  "",
		CreatedAt:    time.Now().UTC().Unix(),
	}

	return chunk
}

func extractChunkText(content []byte, startByte uint32, endByte uint32) string {
	if len(content) == 0 {
		return ""
	}

	if startByte >= uint32(len(content)) || endByte > uint32(len(content)) || startByte >= endByte {
		return string(content)
	}

	return string(content[startByte:endByte])
}

func byteRangeToLineRange(content []byte, startByte uint32, endByte uint32) (int, int) {
	if len(content) == 0 {
		return 0, 0
	}

	if startByte >= uint32(len(content)) || endByte > uint32(len(content)) || startByte >= endByte {
		return 1, strings.Count(string(content), "\n") + 1
	}

	startLine := 1
	for _, b := range content[:startByte] {
		if b == '\n' {
			startLine++
		}
	}

	endLine := startLine
	for _, b := range content[startByte:endByte] {
		if b == '\n' {
			endLine++
		}
	}

	return startLine, endLine
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

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
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
	sharedutil "github.com/rohandave/tessa-rag/services/shared/util"
)

type ChunkingServiceInput struct {
	Logger               *log.Logger
	BlobStoreRepo        *sharedblobstore.Repo
	ChunkRepo            *ports.ChunkRepo
	KafkaProducer        sharedkafka.Producer
	IndexingTopic        string
	NormalizationService *NormalizationService
	ExtractionService    *ExtractionService
}

type ChunkingService struct {
	logger               *log.Logger
	normalizationService *NormalizationService
	extractionService    *ExtractionService
	chunkRepo            *ports.ChunkRepo
	kafkaProducer        sharedkafka.Producer
	indexingTopic        string
	blobStoreRepo        *sharedblobstore.Repo

	normalizationWorkerCount int
	extractionWorkerCount    int
	fileFetchWorkerCount     int
}

func NewChunkingService(input *ChunkingServiceInput) *ChunkingService {
	return &ChunkingService{
		logger:                   input.Logger,
		blobStoreRepo:            input.BlobStoreRepo,
		normalizationService:     input.NormalizationService,
		chunkRepo:                input.ChunkRepo,
		kafkaProducer:            input.KafkaProducer,
		indexingTopic:            input.IndexingTopic,
		extractionService:        input.ExtractionService,
		normalizationWorkerCount: 5,
		extractionWorkerCount:    5,
		fileFetchWorkerCount:     5,
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
	createdCount := len(changeLogFileContent.Created)
	updatedCount := len(changeLogFileContent.Updated)
	deletedCount := len(changeLogFileContent.Deleted)
	s.logf("decoded change log for snapshot=%s repo=%s branch=%s commit=%s", snapshot.Id, changeLogFileContent.RepoURL, changeLogFileContent.Branch, changeLogFileContent.CommitSHA)
	s.logf("change log file counts snapshot=%s created=%d updated=%d deleted=%d", snapshot.Id, createdCount, updatedCount, deletedCount)

	blobDirectoryURL, err := deriveDirectoryURL(snapshot.ChangeLogURL)
	if err != nil {
		s.logf("failed to derive blob directory from change log url for snapshot=%s: %v", snapshot.Id, err)
		return err
	}

	deletedFileNames := make([]string, 0, len(changeLogFileContent.Deleted)+len(changeLogFileContent.Updated))

	createdFileURLs := make([]string, 0, len(changeLogFileContent.Created)+len(changeLogFileContent.Updated))

	for _, file := range changeLogFileContent.Deleted {
		s.logf("queued deleted file for chunk deletion snapshot=%s path=%s hash=%s size=%d", snapshot.Id, file.Path, file.FileHash, file.FileSize)
		deletedFileNames = append(deletedFileNames, file.FileHash)
	}

	for _, file := range changeLogFileContent.Updated {
		fileURL := blobDirectoryURL + "/" + file.NewFileHash
		s.logf("queued updated file for replacement snapshot=%s path=%s old_hash=%s new_hash=%s url=%s new_size=%d", snapshot.Id, file.Path, file.OldFileHash, file.NewFileHash, fileURL, file.NewFileSize)
		deletedFileNames = append(deletedFileNames, file.OldFileHash)
		createdFileURLs = append(createdFileURLs, fileURL)
	}

	for _, file := range changeLogFileContent.Created {
		fileURL := blobDirectoryURL + "/" + file.FileHash
		s.logf("queued file for fetching snapshot=%s path=%s hash=%s url=%s size=%d", snapshot.Id, file.Path, file.FileHash, fileURL, file.FileSize)
		createdFileURLs = append(createdFileURLs, fileURL)
	}

	s.logf("prepared chunking work snapshot=%s files_to_fetch=%d chunks_to_delete_for_files=%d", snapshot.Id, len(createdFileURLs), len(deletedFileNames))

	if len(deletedFileNames) > 0 {
		s.logf("deleting existing chunks snapshot=%s file_hashes=%d", snapshot.Id, len(deletedFileNames))
		deletedChunkIDs, err := s.chunkRepo.DeleteChunksForFiles(context.Background(), deletedFileNames)
		if err != nil {
			s.logf("failed to delete chunks snapshot=%s file_hashes=%d: %v", snapshot.Id, len(deletedFileNames), err)
			return err
		}
		s.logf("marked existing chunks pending delete snapshot=%s file_hashes=%d chunks=%d", snapshot.Id, len(deletedFileNames), len(deletedChunkIDs))

		for _, chunkID := range deletedChunkIDs {
			err := s.publishChunkIndexingEvent(shareddomain.ChunkIndexingEvent{
				EventType:  shareddomain.ChunkDeleteRequestedEvent,
				ChunkID:    chunkID,
				RepoURL:    snapshot.RepoURL,
				Branch:     snapshot.Branch,
				SnapshotID: snapshot.Id,
			})
			if err != nil {
				s.logf("failed to publish chunk delete event snapshot=%s chunk_id=%s: %v", snapshot.Id, chunkID, err)
				return err
			}
		}
		s.logf("published chunk delete events snapshot=%s chunks=%d", snapshot.Id, len(deletedChunkIDs))
	} else {
		s.logf("no deleted or updated files requiring chunk deletion snapshot=%s", snapshot.Id)
	}

	if len(createdFileURLs) == 0 {
		s.logf("no created or updated files to fetch snapshot=%s; deletion-only change log handled", snapshot.Id)
		return nil
	}

	s.logf("fetching %d created/updated files for snapshot=%s with %d blob workers", len(createdFileURLs), snapshot.Id, s.fileFetchWorkerCount)

	fetchedFilesChannel := make(chan *shareddomain.FileJob)
	normalizedFilesChannel := make(chan *shareddomain.FileJob)
	errCh := make(chan error, 1)

	var normalizationWG sync.WaitGroup
	var extractionWG sync.WaitGroup

	for range s.normalizationWorkerCount {
		normalizationWG.Add(1)
		go s.normalizationWorker(fetchedFilesChannel, normalizedFilesChannel, &normalizationWG, errCh)
	}

	for range s.extractionWorkerCount {
		extractionWG.Add(1)
		go s.extractionWorker(
			normalizedFilesChannel,
			snapshot,
			&extractionWG,
			errCh,
		)
	}

	if err := s.blobStoreRepo.GetFiles(createdFileURLs, fetchedFilesChannel, s.fileFetchWorkerCount); err != nil {
		s.logf("failed while fetching files for snapshot=%s: %v", snapshot.Id, err)
		close(fetchedFilesChannel)
		close(normalizedFilesChannel)
		return err
	}

	s.logf("completed blob fetch stage for snapshot=%s fetched_files=%d", snapshot.Id, len(createdFileURLs))
	close(fetchedFilesChannel)
	normalizationWG.Wait()
	s.logf("completed normalization stage for snapshot=%s", snapshot.Id)
	close(normalizedFilesChannel)
	extractionWG.Wait()
	s.logf("completed extraction and chunk persistence stage for snapshot=%s", snapshot.Id)

	select {
	case workerErr := <-errCh:
		if workerErr != nil {
			s.logf("worker error for snapshot=%s: %v", snapshot.Id, workerErr)
			return workerErr
		}
	default:
	}

	s.logf("finished chunking pipeline for snapshot=%s created_files=%d updated_files=%d deleted_files=%d fetched_files=%d deleted_file_hashes=%d", snapshot.Id, createdCount, updatedCount, deletedCount, len(createdFileURLs), len(deletedFileNames))
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

		s.logf("extracted data path=%s symbols=%d containers=%d imports=%d doc_comments=%d classes=%d functions=%d methods=%d module_level_declarations=%d", fileJob.Path, len(extractedData.Symbols), len(extractedData.Containers), len(extractedData.Imports), len(extractedData.DocComments), len(extractedData.Classes), len(extractedData.Functions), len(extractedData.Methods), len(extractedData.ModuleLevelDeclarations))

		chunks := buildChunks(snapshot, fileJob, extractedData)
		s.logf("built chunks snapshot=%s path=%s chunks=%d", snapshot.Id, fileJob.Path, len(chunks))

		for _, chunk := range chunks {
			err = s.chunkRepo.CreateChunk(context.Background(), &chunk)
			if err != nil {
				s.logf("failed to write chunk to database snapshot=%s path=%s chunk_id=%s symbol=%s type=%s: %v", snapshot.Id, fileJob.Path, chunk.ChunkID, chunk.SymbolName, chunk.SymbolType, err)
				select {
				case errCh <- fmt.Errorf("create chunk %s for %s: %w", chunk.ChunkID, fileJob.Path, err):
				default:
				}
				continue
			}

			err = s.publishChunkIndexingEvent(shareddomain.ChunkIndexingEvent{
				EventType:  shareddomain.ChunkIndexRequestedEvent,
				ChunkID:    chunk.ChunkID,
				RepoURL:    chunk.RepoURL,
				Branch:     chunk.Branch,
				SnapshotID: chunk.SnapshotID,
				FileName:   chunk.FileName,
				FilePath:   chunk.FilePath,
			})
			if err != nil {
				s.logf("failed to publish chunk index event snapshot=%s path=%s chunk_id=%s: %v", snapshot.Id, fileJob.Path, chunk.ChunkID, err)
				select {
				case errCh <- fmt.Errorf("publish chunk index event %s for %s: %w", chunk.ChunkID, fileJob.Path, err):
				default:
				}
				continue
			}

			s.logf("stored extracted chunk snapshot=%s path=%s chunk_id=%s symbol=%s type=%s start_line=%d end_line=%d content_bytes=%d", snapshot.Id, fileJob.Path, chunk.ChunkID, chunk.SymbolName, chunk.SymbolType, chunk.StartLine, chunk.EndLine, len(chunk.Content))
		}
	}
}

func (s *ChunkingService) publishChunkIndexingEvent(event shareddomain.ChunkIndexingEvent) error {
	if s.kafkaProducer == nil {
		return fmt.Errorf("kafka producer is not configured")
	}
	if s.indexingTopic == "" {
		return fmt.Errorf("indexing topic is not configured")
	}

	value, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal chunk indexing event: %w", err)
	}

	return s.kafkaProducer.Produce(s.indexingTopic, []byte(event.ChunkID), value)
}

func buildChunks(snapshot shareddomain.Snapshot, fileJob *shareddomain.FileJob, extractedData *ports.CodeParseResult) []shareddomain.Chunk {
	chunkUnits := collectChunkUnits(extractedData)
	if len(chunkUnits) == 0 {
		chunkID := sharedutil.GenerateUUID()
		endByte := uint32(0)
		if fileJob != nil {
			endByte = uint32(len(fileJob.Content))
		}

		return []shareddomain.Chunk{buildChunk(snapshot, fileJob, chunkID, chunkUnit{
			Name:      "",
			Kind:      "file",
			StartByte: 0,
			EndByte:   endByte,
		})}
	}

	chunks := make([]shareddomain.Chunk, 0, len(chunkUnits))
	for _, unit := range chunkUnits {
		chunkID := sharedutil.GenerateUUID()
		chunks = append(chunks, buildChunk(snapshot, fileJob, chunkID, unit))
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

type chunkUnit struct {
	Name      string
	Kind      string
	StartByte uint32
	EndByte   uint32
}

func collectChunkUnits(extractedData *ports.CodeParseResult) []chunkUnit {
	if extractedData == nil {
		return nil
	}

	units := make([]chunkUnit, 0, len(extractedData.Classes)+len(extractedData.Functions)+len(extractedData.Methods)+len(extractedData.ModuleLevelDeclarations)+len(extractedData.Symbols)+len(extractedData.Containers)+len(extractedData.Imports)+len(extractedData.DocComments))
	seen := make(map[string]struct{})

	appendUnit := func(unit chunkUnit) {
		if unit.StartByte >= unit.EndByte {
			return
		}

		key := fmt.Sprintf("%s:%s:%d:%d", unit.Kind, unit.Name, unit.StartByte, unit.EndByte)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		units = append(units, unit)
	}

	appendDeclarations := func(items []ports.CodeDeclaration) {
		for _, declaration := range items {
			appendUnit(chunkUnit{
				Name:      declaration.Name,
				Kind:      declaration.Kind,
				StartByte: declaration.StartByte,
				EndByte:   declaration.EndByte,
			})
		}
	}

	appendDeclarations(extractedData.Classes)
	appendDeclarations(extractedData.Functions)
	appendDeclarations(extractedData.Methods)
	appendDeclarations(extractedData.ModuleLevelDeclarations)

	for _, symbol := range extractedData.Symbols {
		appendUnit(chunkUnit{
			Name:      symbol.Name,
			Kind:      "symbol:" + symbol.Kind,
			StartByte: symbol.StartByte,
			EndByte:   symbol.EndByte,
		})
	}

	for _, container := range extractedData.Containers {
		appendUnit(chunkUnit{
			Name:      container.Name,
			Kind:      "container:" + container.Kind,
			StartByte: container.StartByte,
			EndByte:   container.EndByte,
		})
	}

	for _, item := range extractedData.Imports {
		appendUnit(chunkUnit{
			Name:      item.Path,
			Kind:      "import",
			StartByte: item.StartByte,
			EndByte:   item.EndByte,
		})
	}

	for _, comment := range extractedData.DocComments {
		name := comment.Target
		if name == "" {
			name = "doc_comment"
		}

		appendUnit(chunkUnit{
			Name:      name,
			Kind:      "doc_comment",
			StartByte: comment.StartByte,
			EndByte:   comment.EndByte,
		})
	}

	return units
}

func buildChunk(snapshot shareddomain.Snapshot, fileJob *shareddomain.FileJob, chunkID string, unit chunkUnit) shareddomain.Chunk {
	filePath := ""
	fileName := ""
	language := "unknown"
	content := ""
	startByte := unit.StartByte
	endByte := unit.EndByte
	startLine := 0
	endLine := 0

	if fileJob != nil {
		filePath = fileJob.Path
		fileName = path.Base(fileJob.Path)
		language = normalizeLanguage(fileJob.Extension)
		content = extractChunkText(fileJob.Content, startByte, endByte)
		startLine, endLine = byteRangeToLineRange(fileJob.Content, startByte, endByte)
	}

	chunk := shareddomain.Chunk{
		ChunkID:                chunkID,
		RepoURL:                snapshot.RepoURL,
		FileName:               fileName,
		Branch:                 snapshot.Branch,
		CommitSHA:              snapshot.CommitSHA,
		SnapshotID:             snapshot.Id,
		FilePath:               filePath,
		DocumentType:           "code",
		Language:               language,
		Content:                content,
		ContentHash:            sharedutil.HashString(content),
		SymbolName:             unit.Name,
		SymbolType:             unit.Kind,
		StartLine:              startLine,
		EndLine:                endLine,
		StartChar:              int(startByte),
		EndChar:                int(endByte),
		Status:                 "pending_index",
		TextSearchEngineStatus: "pending",
		VectorDbStatus:         "pending",
		PrevChunkID:            "pending",
		NextChunkID:            "pending",
		CreatedAt:              time.Now().UTC().Unix(),
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

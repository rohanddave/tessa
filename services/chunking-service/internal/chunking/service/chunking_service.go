package service

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	sharedblobstore "github.com/rohandave/tessa-rag/services/shared/blobstore"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
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
	s.logger.Printf("starting chunking pipeline for snapshot=%s repo=%s changelog=%s", snapshot.Id, snapshot.RepoURL, snapshot.ChangeLogURL)

	changeLogFile, err := s.blobStoreRepo.GetFile(snapshot.ChangeLogURL)
	if err != nil {
		s.logger.Printf("failed to fetch change log for snapshot=%s: %v", snapshot.Id, err)
		return err
	}
	s.logger.Printf("loaded change log for snapshot=%s bytes=%d extension=%s", snapshot.Id, len(changeLogFile.Content), changeLogFile.Extension)

	var changeLogFileContent shareddomain.ChangeLog
	err = json.Unmarshal(changeLogFile.Content, &changeLogFileContent)
	if err != nil {
		s.logger.Printf("failed to decode change log for snapshot=%s: %v", snapshot.Id, err)
		return err
	}
	createdCount := len(changeLogFileContent.Created)
	updatedCount := len(changeLogFileContent.Updated)
	deletedCount := len(changeLogFileContent.Deleted)
	s.logger.Printf("decoded change log for snapshot=%s repo=%s branch=%s commit=%s", snapshot.Id, changeLogFileContent.RepoURL, changeLogFileContent.Branch, changeLogFileContent.CommitSHA)
	s.logger.Printf("change log file counts snapshot=%s created=%d updated=%d deleted=%d", snapshot.Id, createdCount, updatedCount, deletedCount)

	blobDirectoryURL, err := deriveDirectoryURL(snapshot.ChangeLogURL)
	if err != nil {
		s.logger.Printf("failed to derive blob directory from change log url for snapshot=%s: %v", snapshot.Id, err)
		return err
	}

	deletedFileNames := make([]string, 0, len(changeLogFileContent.Deleted)+len(changeLogFileContent.Updated))
	createdFileURLs := make([]string, 0, len(changeLogFileContent.Created)+len(changeLogFileContent.Updated))

	for _, file := range changeLogFileContent.Deleted {
		s.logger.Printf("queued deleted file for chunk deletion snapshot=%s path=%s hash=%s size=%d", snapshot.Id, file.Path, file.FileHash, file.FileSize)
		deletedFileNames = append(deletedFileNames, file.FileHash)
	}

	for _, file := range changeLogFileContent.Updated {
		fileURL := blobDirectoryURL + "/" + file.NewFileHash
		s.logger.Printf("queued updated file for replacement snapshot=%s path=%s old_hash=%s new_hash=%s url=%s new_size=%d", snapshot.Id, file.Path, file.OldFileHash, file.NewFileHash, fileURL, file.NewFileSize)
		deletedFileNames = append(deletedFileNames, file.OldFileHash)
		createdFileURLs = append(createdFileURLs, fileURL)
	}

	for _, file := range changeLogFileContent.Created {
		fileURL := blobDirectoryURL + "/" + file.FileHash
		s.logger.Printf("queued file for fetching snapshot=%s path=%s hash=%s url=%s size=%d", snapshot.Id, file.Path, file.FileHash, fileURL, file.FileSize)
		createdFileURLs = append(createdFileURLs, fileURL)
	}

	s.logger.Printf("prepared chunking work snapshot=%s files_to_fetch=%d chunks_to_delete_for_files=%d", snapshot.Id, len(createdFileURLs), len(deletedFileNames))

	err = NewDeletedFileChunkProcessor(&DeletedFileChunkProcessorInput{
		snapshot:           &snapshot,
		logger:             s.logger,
		chunkRepo:          s.chunkRepo,
		kafkaProducer:      s.kafkaProducer,
		kafkaIndexingTopic: s.indexingTopic,
	}).Run(deletedFileNames)

	err = NewCreatedFileChunkProcessor(&CreatedFileChunkProcessorInput{
		snapshot:             &snapshot,
		logger:               s.logger,
		blobStoreRepo:        s.blobStoreRepo,
		normalizationService: s.normalizationService,
		extractionService:    s.extractionService,
		chunkRepo:            s.chunkRepo,
		kafkaProducer:        s.kafkaProducer,
		kafkaIndexingTopic:   s.indexingTopic,
	}).Run(createdFileURLs)

	return nil
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

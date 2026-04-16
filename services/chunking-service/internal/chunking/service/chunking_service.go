package service

import (
	"context"
	"encoding/json"
	"log"
	"slices"

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
		fileFetchWorkerCount:     20,
	}
}

func (s *ChunkingService) Start(snapshot shareddomain.Snapshot) (err error) {
	s.logger.Printf("starting chunking pipeline for snapshot=%s repo=%s changelog=%s", snapshot.Id, snapshot.RepoURL, snapshot.ChangeLogURL)

	ctx := context.Background()
	claimed, err := s.chunkRepo.TryStartSnapshotChunking(ctx, &snapshot)
	if err != nil {
		return err
	}
	if !claimed {
		s.logger.Printf("skipping snapshot chunking because job already completed snapshot=%s repo=%s", snapshot.Id, snapshot.RepoURL)
		return nil
	}

	defer func() {
		if err == nil {
			return
		}
		if markErr := s.chunkRepo.MarkSnapshotChunkingFailed(context.Background(), snapshot.Id); markErr != nil {
			s.logger.Printf("failed to mark snapshot chunking failed snapshot=%s: %v", snapshot.Id, markErr)
		}
	}()

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

	changeLogFileContent.Created = slices.Grow(changeLogFileContent.Created, updatedCount)
	changeLogFileContent.Deleted = slices.Grow(changeLogFileContent.Deleted, updatedCount)

	for i := range changeLogFileContent.Updated {
		u := changeLogFileContent.Updated[i]
		changeLogFileContent.Created = append(changeLogFileContent.Created, shareddomain.ChangeLogFile{
			Path:          u.Path,
			FileHash:      u.NewFileHash,
			FileSize:      u.NewFileSize,
			FileExtension: u.FileExtension,
		})

		changeLogFileContent.Deleted = append(changeLogFileContent.Deleted, shareddomain.ChangeLogFile{
			Path:          u.Path,
			FileHash:      u.OldFileHash,
			FileSize:      u.OldFileSize,
			FileExtension: u.FileExtension,
		})
	}

	changeLogFileContent.Updated = nil

	err = NewDeletedFileChunkProcessor(&DeletedFileChunkProcessorInput{
		snapshot:           &snapshot,
		logger:             s.logger,
		chunkRepo:          s.chunkRepo,
		kafkaProducer:      s.kafkaProducer,
		kafkaIndexingTopic: s.indexingTopic,
	}).Run(changeLogFileContent.Deleted)
	if err != nil {
		s.logger.Printf("error deleting files snapshot=%s: %v", snapshot.Id, err)
		return err
	}

	err = NewCreatedFileChunkProcessor(&CreatedFileChunkProcessorInput{
		snapshot:             &snapshot,
		logger:               s.logger,
		blobStoreRepo:        s.blobStoreRepo,
		normalizationService: s.normalizationService,
		extractionService:    s.extractionService,
		chunkRepo:            s.chunkRepo,
		kafkaProducer:        s.kafkaProducer,
		kafkaIndexingTopic:   s.indexingTopic,
	}).Run(changeLogFileContent.Created)
	if err != nil {
		s.logger.Printf("error creating files snapshot=%s: %v", snapshot.Id, err)
		return err
	}

	if err = s.chunkRepo.MarkSnapshotChunkingCompleted(ctx, snapshot.Id); err != nil {
		s.logger.Printf("error marking snapshot with id=%s as completed: %v", snapshot.Id, err)
		return err
	}

	s.logger.Printf("successfully completed chunking snapshot with id=%s", snapshot.Id)
	return nil
}

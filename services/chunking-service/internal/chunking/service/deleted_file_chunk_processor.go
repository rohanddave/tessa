package service

import (
	"context"
	"encoding/json"
	"log"

	"github.com/rohandave/tessa-rag/services/chunking-service/internal/chunking/ports"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedkafka "github.com/rohandave/tessa-rag/services/shared/kafka"
)

type DeletedFileChunkProcessor struct {
	snapshot *shareddomain.Snapshot

	logger *log.Logger

	chunkRepo *ports.ChunkRepo

	kafkaProducer      sharedkafka.Producer
	kafkaIndexingTopic string
}

type DeletedFileChunkProcessorInput struct {
	snapshot           *shareddomain.Snapshot
	logger             *log.Logger
	chunkRepo          *ports.ChunkRepo
	kafkaProducer      sharedkafka.Producer
	kafkaIndexingTopic string
}

func NewDeletedFileChunkProcessor(input *DeletedFileChunkProcessorInput) *DeletedFileChunkProcessor {
	return &DeletedFileChunkProcessor{
		snapshot: input.snapshot,
		logger:   input.logger,

		chunkRepo: input.chunkRepo,

		kafkaProducer:      input.kafkaProducer,
		kafkaIndexingTopic: input.kafkaIndexingTopic,
	}
}

func (s *DeletedFileChunkProcessor) Run(changeLogFiles []shareddomain.ChangeLogFile) error {
	if len(changeLogFiles) == 0 {
		// no files to process
		s.logger.Printf("no files to delete")
		return nil
	}

	filePaths := make([]string, 0, len(changeLogFiles))

	for _, file := range changeLogFiles {
		s.logger.Printf("queued deleted file for chunk deletion snapshot=%s path=%s hash=%s size=%d", s.snapshot.Id, file.Path, file.FileHash, file.FileSize)
		filePaths = append(filePaths, file.Path)
	}

	deletedChunkIDs, err := s.chunkRepo.DeleteChunksForFiles(context.Background(), filePaths)
	if err != nil {
		s.logger.Printf("failed to delete chunks snapshot=%s file_hashes=%d: %v", s.snapshot.Id, len(filePaths), err)
		return err
	}

	// TODO: check if chunks were correctly deleted; there could be a case where DeleteChunksForFiles does not return an error but does not find any chunks to delete - we should either log this or have some sort of check
	s.logger.Printf("marked existing chunks pending delete snapshot=%s file_hashes=%d chunks=%d", s.snapshot.Id, len(filePaths), len(deletedChunkIDs))
	for _, chunkID := range deletedChunkIDs {
		event := shareddomain.ChunkIndexingEvent{
			EventType:  shareddomain.ChunkDeleteRequestedEvent,
			ChunkID:    chunkID,
			RepoURL:    s.snapshot.RepoURL,
			Branch:     s.snapshot.Branch,
			SnapshotID: s.snapshot.Id,
		}
		value, err := json.Marshal(event)
		if err != nil {
			s.logger.Printf("failed to marshal chunk indexing event chunk_id=%s: %v", event.ChunkID, err)
			continue
		}
		if err := s.kafkaProducer.Produce(s.kafkaIndexingTopic, []byte(event.ChunkID), value); err != nil {
			s.logger.Printf("failed to produce indexing event chunk_id=%s: %v", event.ChunkID, err)
			continue
		}
	}
	s.logger.Printf("published chunk delete events snapshot=%s chunks=%d", s.snapshot.Id, len(deletedChunkIDs))
	return nil
}

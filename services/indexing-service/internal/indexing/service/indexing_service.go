package service

import (
	"context"
	"fmt"
	"log"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/indexing/ports"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

type IndexingService struct {
	logger      *log.Logger
	chunkRepo   *ports.ChunkRepo
	textSearch  ports.TextSearchIndexer
	vectorStore ports.VectorIndexer
	graphStore  ports.GraphIndexer
}

func NewIndexingService(logger *log.Logger, chunkRepo *ports.ChunkRepo, textSearch ports.TextSearchIndexer, vectorStore ports.VectorIndexer, graphStore ports.GraphIndexer) *IndexingService {
	return &IndexingService{
		logger:      logger,
		chunkRepo:   chunkRepo,
		textSearch:  textSearch,
		vectorStore: vectorStore,
		graphStore:  graphStore,
	}
}

func (s *IndexingService) HandleChunkEvent(ctx context.Context, event domain.ChunkIndexingEvent) error {
	s.logf("handling chunk indexing event type=%s chunk_id=%s snapshot=%s", event.EventType, event.ChunkID, event.SnapshotID)

	switch event.EventType {
	case domain.ChunkIndexRequestedEvent:
		return s.indexChunk(ctx, event.ChunkID)
	case domain.ChunkDeleteRequestedEvent:
		return s.deleteChunk(ctx, event.ChunkID)
	default:
		return fmt.Errorf("unsupported chunk indexing event type %q", event.EventType)
	}
}

func (s *IndexingService) indexChunk(ctx context.Context, chunkID string) error {
	chunk, err := s.chunkRepo.GetChunkByID(ctx, chunkID)
	if err != nil {
		return err
	}

	if err := s.textSearch.IndexChunk(ctx, chunk); err != nil {
		return fmt.Errorf("index chunk %s in elasticsearch: %w", chunkID, err)
	}
	if err := s.chunkRepo.MarkTextSearchIndexed(ctx, chunkID); err != nil {
		return err
	}

	if err := s.vectorStore.IndexChunk(ctx, chunk); err != nil {
		return fmt.Errorf("index chunk %s in pinecone: %w", chunkID, err)
	}
	if err := s.chunkRepo.MarkVectorIndexed(ctx, chunkID); err != nil {
		return err
	}

	if err := s.graphStore.IndexChunk(ctx, chunk); err != nil {
		return fmt.Errorf("index chunk %s in neo4j: %w", chunkID, err)
	}
	if err := s.chunkRepo.MarkGraphIndexed(ctx, chunkID); err != nil {
		return err
	}

	if err := s.chunkRepo.MarkChunkIndexed(ctx, chunkID); err != nil {
		return err
	}

	s.logf("indexed chunk chunk_id=%s file=%s symbol=%s type=%s", chunkID, chunk.FilePath, chunk.SymbolName, chunk.SymbolType)
	return nil
}

func (s *IndexingService) deleteChunk(ctx context.Context, chunkID string) error {
	if err := s.textSearch.DeleteChunk(ctx, chunkID); err != nil {
		return fmt.Errorf("delete chunk %s from elasticsearch: %w", chunkID, err)
	}
	if err := s.chunkRepo.MarkTextSearchDeleted(ctx, chunkID); err != nil {
		return err
	}

	if err := s.vectorStore.DeleteChunk(ctx, chunkID); err != nil {
		return fmt.Errorf("delete chunk %s from pinecone: %w", chunkID, err)
	}
	if err := s.chunkRepo.MarkVectorDeleted(ctx, chunkID); err != nil {
		return err
	}

	if err := s.graphStore.DeleteChunk(ctx, chunkID); err != nil {
		return fmt.Errorf("delete chunk %s from neo4j: %w", chunkID, err)
	}
	if err := s.chunkRepo.MarkGraphDeleted(ctx, chunkID); err != nil {
		return err
	}

	if err := s.chunkRepo.MarkChunkDeleted(ctx, chunkID); err != nil {
		return err
	}

	s.logf("deleted chunk from indexes chunk_id=%s", chunkID)
	return nil
}

func (s *IndexingService) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

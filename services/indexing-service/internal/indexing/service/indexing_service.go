package service

import (
	"context"
	"fmt"
	"log"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/indexing-service/internal/indexing/ports"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

type TextSearchIndexer interface {
	IndexChunk(ctx context.Context, chunk *domain.Chunk) error
	DeleteChunk(ctx context.Context, chunkID string) error
}

type VectorIndexer interface {
	IndexChunk(ctx context.Context, chunk *domain.Chunk) error
	DeleteChunk(ctx context.Context, chunkID string) error
}

type GraphIndexer interface {
	IndexChunk(ctx context.Context, chunk *domain.Chunk) error
	DeleteChunk(ctx context.Context, chunkID string) error
}

type IndexingService struct {
	logger      *log.Logger
	chunkRepo   *ports.ChunkRepo
	textSearch  TextSearchIndexer
	vectorStore VectorIndexer
	graphStore  GraphIndexer
}

func NewIndexingService(logger *log.Logger, chunkRepo *ports.ChunkRepo, textSearch TextSearchIndexer, vectorStore VectorIndexer, graphStore GraphIndexer) *IndexingService {
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
	if err := s.vectorStore.IndexChunk(ctx, chunk); err != nil {
		return fmt.Errorf("index chunk %s in pinecone: %w", chunkID, err)
	}
	if err := s.graphStore.IndexChunk(ctx, chunk); err != nil {
		return fmt.Errorf("index chunk %s in neo4j: %w", chunkID, err)
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
	if err := s.vectorStore.DeleteChunk(ctx, chunkID); err != nil {
		return fmt.Errorf("delete chunk %s from pinecone: %w", chunkID, err)
	}
	if err := s.graphStore.DeleteChunk(ctx, chunkID); err != nil {
		return fmt.Errorf("delete chunk %s from neo4j: %w", chunkID, err)
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

type ElasticsearchIndexer struct {
	logger *log.Logger
	config *config.ElasticsearchConfig
}

func NewElasticsearchIndexer(logger *log.Logger, cfg *config.ElasticsearchConfig) *ElasticsearchIndexer {
	return &ElasticsearchIndexer{logger: logger, config: cfg}
}

func (i *ElasticsearchIndexer) IndexChunk(ctx context.Context, chunk *domain.Chunk) error {
	i.logger.Printf("elasticsearch index requested url=%s chunk_id=%s content_hash=%s", i.config.URL, chunk.ChunkID, chunk.ContentHash)
	return nil
}

func (i *ElasticsearchIndexer) DeleteChunk(ctx context.Context, chunkID string) error {
	i.logger.Printf("elasticsearch delete requested url=%s chunk_id=%s", i.config.URL, chunkID)
	return nil
}

type PineconeIndexer struct {
	logger *log.Logger
	config *config.PineconeConfig
}

func NewPineconeIndexer(logger *log.Logger, cfg *config.PineconeConfig) *PineconeIndexer {
	return &PineconeIndexer{logger: logger, config: cfg}
}

func (i *PineconeIndexer) IndexChunk(ctx context.Context, chunk *domain.Chunk) error {
	i.logger.Printf("pinecone index requested host=%s index=%s chunk_id=%s", i.config.Host, i.config.Index, chunk.ChunkID)
	return nil
}

func (i *PineconeIndexer) DeleteChunk(ctx context.Context, chunkID string) error {
	i.logger.Printf("pinecone delete requested host=%s index=%s chunk_id=%s", i.config.Host, i.config.Index, chunkID)
	return nil
}

type Neo4jIndexer struct {
	logger *log.Logger
	config *config.Neo4jConfig
}

func NewNeo4jIndexer(logger *log.Logger, cfg *config.Neo4jConfig) *Neo4jIndexer {
	return &Neo4jIndexer{logger: logger, config: cfg}
}

func (i *Neo4jIndexer) IndexChunk(ctx context.Context, chunk *domain.Chunk) error {
	i.logger.Printf("neo4j index requested uri=%s chunk_id=%s symbol=%s", i.config.URI, chunk.ChunkID, chunk.SymbolName)
	return nil
}

func (i *Neo4jIndexer) DeleteChunk(ctx context.Context, chunkID string) error {
	i.logger.Printf("neo4j delete requested uri=%s chunk_id=%s", i.config.URI, chunkID)
	return nil
}

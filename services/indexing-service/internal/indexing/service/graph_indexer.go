package service

import (
	"context"
	"log"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

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

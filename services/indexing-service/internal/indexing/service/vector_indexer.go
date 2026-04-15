package service

import (
	"context"
	"log"

	"github.com/rohandave/tessa-rag/services/indexing-service/internal/config"
	"github.com/rohandave/tessa-rag/services/shared/domain"
)

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

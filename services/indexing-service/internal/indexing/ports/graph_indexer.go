package ports

import (
	"context"

	"github.com/rohandave/tessa-rag/services/shared/domain"
)

type GraphIndexer interface {
	IndexChunk(ctx context.Context, chunk *domain.Chunk) error
	DeleteChunk(ctx context.Context, chunkID string) error
}

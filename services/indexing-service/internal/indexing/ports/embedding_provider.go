package ports

import "context"

type EmbeddingProvider interface {
	Embed(ctx context.Context, input string) ([]float32, error)
}

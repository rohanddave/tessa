package ports

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohandave/tessa-rag/services/shared/domain"
	sharedpostgres "github.com/rohandave/tessa-rag/services/shared/postgres"
)

type ChunkRepo struct {
	pool *pgxpool.Pool
}

func NewChunkRepo(ctx context.Context, cfg *sharedpostgres.DatabaseConfig) (*ChunkRepo, error) {
	dsn := fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Name,
		cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	return &ChunkRepo{pool: pool}, nil
}

func (r *ChunkRepo) Close() {
	r.pool.Close()
}

func (r *ChunkRepo) GetChunkByID(ctx context.Context, chunkID string) (*domain.Chunk, error) {
	const query = `
		SELECT
			chunk_id,
			repo_url,
			file_name,
			branch,
			commit_sha,
			snapshot_id,
			file_path,
			document_type,
			language,
			content,
			content_hash,
			symbol_name,
			symbol_type,
			start_line,
			end_line,
			start_char,
			end_char,
			prev_chunk_id,
			next_chunk_id,
			status,
			elasticsearch_status,
			pinecone_status,
			neo4j_status,
			created_at
		FROM chunks
		WHERE chunk_id = $1
	`

	var chunk domain.Chunk
	err := r.pool.QueryRow(ctx, query, chunkID).Scan(
		&chunk.ChunkID,
		&chunk.RepoURL,
		&chunk.FileName,
		&chunk.Branch,
		&chunk.CommitSHA,
		&chunk.SnapshotID,
		&chunk.FilePath,
		&chunk.DocumentType,
		&chunk.Language,
		&chunk.Content,
		&chunk.ContentHash,
		&chunk.SymbolName,
		&chunk.SymbolType,
		&chunk.StartLine,
		&chunk.EndLine,
		&chunk.StartChar,
		&chunk.EndChar,
		&chunk.PrevChunkID,
		&chunk.NextChunkID,
		&chunk.Status,
		&chunk.TextSearchEngineStatus,
		&chunk.VectorDbStatus,
		&chunk.GraphDbStatus,
		&chunk.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get chunk %s: %w", chunkID, err)
	}

	return &chunk, nil
}

func (r *ChunkRepo) MarkChunkIndexed(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET
			status = 'indexed'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s indexed: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkTextSearchIndexed(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET elasticsearch_status = 'indexed'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s indexed in elasticsearch: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkVectorIndexed(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET pinecone_status = 'indexed'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s indexed in pinecone: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkGraphIndexed(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET neo4j_status = 'indexed'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s indexed in neo4j: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkChunkDeleted(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET
			status = 'deleted'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s deleted: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkTextSearchDeleted(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET elasticsearch_status = 'deleted'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s deleted from elasticsearch: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkVectorDeleted(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET pinecone_status = 'deleted'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s deleted from pinecone: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkGraphDeleted(ctx context.Context, chunkID string) error {
	const query = `
		UPDATE chunks
		SET neo4j_status = 'deleted'
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("mark chunk %s deleted from neo4j: %w", chunkID, err)
	}

	return nil
}

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

	repo := &ChunkRepo{
		pool: pool,
	}

	if err := repo.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return repo, nil
}

func (r *ChunkRepo) CreateChunk(ctx context.Context, chunk *domain.Chunk) error {
	const query = `
		INSERT INTO chunks (
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
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
	`

	_, err := r.pool.Exec(
		ctx,
		query,
		chunk.ChunkID,
		chunk.RepoURL,
		chunk.FileName,
		chunk.Branch,
		chunk.CommitSHA,
		chunk.SnapshotID,
		chunk.FilePath,
		chunk.DocumentType,
		chunk.Language,
		chunk.Content,
		chunk.ContentHash,
		chunk.SymbolName,
		chunk.SymbolType,
		chunk.StartLine,
		chunk.EndLine,
		chunk.StartChar,
		chunk.EndChar,
		chunk.PrevChunkID,
		chunk.NextChunkID,
		chunk.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create chunk %s: %w", chunk.ChunkID, err)
	}

	return nil
}

func (r *ChunkRepo) ListChunksBySnapshotID(ctx context.Context, snapshotID string) ([]domain.Chunk, error) {
	const query = `
		SELECT *
		FROM chunks
		WHERE snapshot_id = $1
		ORDER BY file_path, start_line, start_char
	`

	rows, err := r.pool.Query(ctx, query, snapshotID)
	if err != nil {
		return nil, fmt.Errorf("list chunks by snapshot id %s: %w", snapshotID, err)
	}
	defer rows.Close()

	var chunks []domain.Chunk
	for rows.Next() {
		var chunk domain.Chunk
		if err := rows.Scan(
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
			&chunk.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan chunk row: %w", err)
		}
		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chunk rows: %w", err)
	}

	return chunks, nil
}

func (r *ChunkRepo) DeleteChunk(ctx context.Context, chunkID string) error {
	const query = `
		DELETE FROM chunks
		WHERE chunk_id = $1
	`

	_, err := r.pool.Exec(ctx, query, chunkID)
	if err != nil {
		return fmt.Errorf("delete chunk %s: %w", chunkID, err)
	}

	return nil
}

func (r *ChunkRepo) DeleteChunksForFile(ctx context.Context, fileName string) error {
	const query = `
		DELETE FROM chunks
		WHERE file_name = $1
	`

	_, err := r.pool.Exec(ctx, query, fileName)
	if err != nil {
		return fmt.Errorf("delete chunks for file with hash %s: %w", fileName, err)
	}

	return nil
}

func (r *ChunkRepo) ensureSchema(ctx context.Context) error {
	_, err := r.pool.Exec(
		ctx,
		`
		CREATE TABLE IF NOT EXISTS chunks (
			chunk_id TEXT PRIMARY KEY,
			repo_url TEXT NOT NULL,
			file_name TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT '',
			commit_sha TEXT NOT NULL DEFAULT '',
			snapshot_id TEXT NOT NULL,
			file_path TEXT NOT NULL,
			document_type TEXT NOT NULL DEFAULT '',
			language TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			symbol_name TEXT NOT NULL DEFAULT '',
			symbol_type TEXT NOT NULL DEFAULT '',
			start_line INTEGER NOT NULL,
			end_line INTEGER NOT NULL,
			start_char INTEGER NOT NULL,
			end_char INTEGER NOT NULL,
			prev_chunk_id TEXT NOT NULL DEFAULT '',
			next_chunk_id TEXT NOT NULL DEFAULT '',
			created_at BIGINT NOT NULL
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure snapshots schema: %w", err)
	}

	return nil
}

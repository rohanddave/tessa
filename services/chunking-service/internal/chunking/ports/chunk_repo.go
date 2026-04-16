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

const (
	defaultChunkStatus         = "pending_index"
	defaultTargetIndexStatus   = "pending"
	defaultTargetDeleteStatus  = "pending_delete"
	chunkStatusPendingDelete   = "pending_delete"
	snapshotStatusCreated      = "created"
	snapshotStatusChunking     = "chunking"
	snapshotStatusChunked      = "chunked"
	snapshotChunkingStatusFailed = "chunking_failed"
)

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
	status := defaultString(chunk.Status, defaultChunkStatus)
	textSearchEngineStatus := defaultString(chunk.TextSearchEngineStatus, defaultTargetIndexStatus)
	vectorDbStatus := defaultString(chunk.VectorDbStatus, defaultTargetIndexStatus)
	graphDbStatus := defaultString(chunk.GraphDbStatus, defaultTargetIndexStatus)

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
			status,
			elasticsearch_status,
			pinecone_status,
			neo4j_status,
			created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24
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
		status,
		textSearchEngineStatus,
		vectorDbStatus,
		graphDbStatus,
		chunk.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create chunk %s: %w", chunk.ChunkID, err)
	}

	return nil
}

func (r *ChunkRepo) TryStartSnapshotChunking(ctx context.Context, snapshot *domain.Snapshot) (bool, error) {
	if snapshot == nil {
		return false, fmt.Errorf("snapshot cannot be nil")
	}

	const query = `
		UPDATE snapshots
		SET status = $2
		WHERE id = $1
			AND status = ANY($3)
	`

	commandTag, err := r.pool.Exec(
		ctx,
		query,
		snapshot.Id,
		snapshotStatusChunking,
		[]string{snapshotStatusCreated, snapshotChunkingStatusFailed},
	)
	if err != nil {
		return false, fmt.Errorf("claim snapshot chunking %s: %w", snapshot.Id, err)
	}

	if commandTag.RowsAffected() > 0 {
		return true, nil
	}

	status, err := r.GetSnapshotChunkingStatus(ctx, snapshot.Id)
	if err != nil {
		return false, err
	}
	if IsSnapshotChunkingCompletedStatus(status) {
		return false, nil
	}

	return false, fmt.Errorf("snapshot chunking already claimed snapshot=%s status=%s", snapshot.Id, status)
}

func (r *ChunkRepo) GetSnapshotChunkingStatus(ctx context.Context, snapshotID string) (string, error) {
	const query = `
		SELECT status
		FROM snapshots
		WHERE id = $1
	`

	var status string
	err := r.pool.QueryRow(ctx, query, snapshotID).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("get snapshot status %s: %w", snapshotID, err)
	}

	return status, nil
}

func IsSnapshotChunkingCompletedStatus(status string) bool {
	return status == snapshotStatusChunked
}

func (r *ChunkRepo) MarkSnapshotChunkingCompleted(ctx context.Context, snapshotID string) error {
	const query = `
		UPDATE snapshots
		SET status = $2
		WHERE id = $1
	`

	_, err := r.pool.Exec(ctx, query, snapshotID, snapshotStatusChunked)
	if err != nil {
		return fmt.Errorf("mark snapshot chunking completed %s: %w", snapshotID, err)
	}

	return nil
}

func (r *ChunkRepo) MarkSnapshotChunkingFailed(ctx context.Context, snapshotID string) error {
	const query = `
		UPDATE snapshots
		SET status = $2
		WHERE id = $1
	`

	_, err := r.pool.Exec(ctx, query, snapshotID, snapshotChunkingStatusFailed)
	if err != nil {
		return fmt.Errorf("mark snapshot chunking failed %s: %w", snapshotID, err)
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
			&chunk.Status,
			&chunk.TextSearchEngineStatus,
			&chunk.VectorDbStatus,
			&chunk.GraphDbStatus,
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

func (r *ChunkRepo) DeleteChunk(ctx context.Context, chunkID string) ([]string, error) {
	const query = `
		UPDATE chunks
		SET
			status = $2,
			elasticsearch_status = $3,
			pinecone_status = $4,
			neo4j_status = $5
		WHERE chunk_id = $1
		RETURNING chunk_id
	`

	return r.markChunksPendingDelete(ctx, query, chunkID)
}

func (r *ChunkRepo) DeleteChunksForFile(ctx context.Context, fileName string) ([]string, error) {
	const query = `
		UPDATE chunks
		SET
			status = $2,
			elasticsearch_status = $3,
			pinecone_status = $4,
			neo4j_status = $5
		WHERE file_name = $1
		RETURNING chunk_id
	`

	return r.markChunksPendingDelete(ctx, query, fileName)
}

func (r *ChunkRepo) DeleteChunksForFiles(ctx context.Context, fileNames []string) ([]string, error) {
	if len(fileNames) == 0 {
		return nil, nil // nothing to delete
	}

	const query = `
		UPDATE chunks
		SET
			status = $2,
			elasticsearch_status = $3,
			pinecone_status = $4,
			neo4j_status = $5
		WHERE file_name = ANY($1)
		RETURNING chunk_id
	`

	return r.markChunksPendingDelete(ctx, query, fileNames)
}

func (r *ChunkRepo) markChunksPendingDelete(ctx context.Context, query string, target any) ([]string, error) {
	rows, err := r.pool.Query(ctx, query, target, chunkStatusPendingDelete, defaultTargetDeleteStatus, defaultTargetDeleteStatus, defaultTargetDeleteStatus)
	if err != nil {
		return nil, fmt.Errorf("mark chunks pending delete for %v: %w", target, err)
	}
	defer rows.Close()

	chunkIDs := make([]string, 0)
	for rows.Next() {
		var chunkID string
		if err := rows.Scan(&chunkID); err != nil {
			return nil, fmt.Errorf("scan pending delete chunk id for %v: %w", target, err)
		}
		chunkIDs = append(chunkIDs, chunkID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending delete chunk ids for %v: %w", target, err)
	}

	return chunkIDs, nil
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
			status TEXT NOT NULL DEFAULT 'pending_index',
			elasticsearch_status TEXT NOT NULL DEFAULT 'pending',
			pinecone_status TEXT NOT NULL DEFAULT 'pending',
			neo4j_status TEXT NOT NULL DEFAULT 'pending',
			created_at BIGINT NOT NULL
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure snapshots schema: %w", err)
	}

	_, err = r.pool.Exec(
		ctx,
		`
		CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			repo_url TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT '',
			commit_sha TEXT NOT NULL DEFAULT '',
			manifest_url TEXT NOT NULL,
			change_log_url TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'created'
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure snapshots schema for chunking status: %w", err)
	}

	_, err = r.pool.Exec(
		ctx,
		`
		ALTER TABLE snapshots
		ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'created'
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure snapshots status column: %w", err)
	}

	return nil
}

func defaultString(value string, fallback string) string {
	if value == "" {
		return fallback
	}

	return value
}

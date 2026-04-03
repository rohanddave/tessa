package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/domain"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
)

type SnapshotStoreRepo struct {
	pool *pgxpool.Pool
}

func NewSnapshotStoreRepo(ctx context.Context, cfg config.DatabaseConfig) (ports.SnapshotStoreRepo, error) {
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

	repo := &SnapshotStoreRepo{
		pool: pool,
	}

	if err := repo.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return repo, nil
}

func (r *SnapshotStoreRepo) CreateSnapshot(snapshot *domain.Snapshot) (string, error) {
	id := snapshot.Id
	if id == "" {
		id = uuid.NewString()
	}

	_, err := r.pool.Exec(
		context.Background(),
		`
		INSERT INTO snapshots (id, repo_url, branch, commit_sha, manifest_url)
		VALUES ($1, $2, $3, $4, $5)
		`,
		id,
		snapshot.RepoURL,
		snapshot.Branch,
		snapshot.CommitSHA,
		snapshot.ManifestURL,
	)
	if err != nil {
		return "", fmt.Errorf("insert snapshot: %w", err)
	}

	return id, nil
}

func (r *SnapshotStoreRepo) GetSnapshot(snapshotID string) (*domain.Snapshot, error) {
	var snapshot domain.Snapshot

	err := r.pool.QueryRow(
		context.Background(),
		`
		SELECT id, repo_url, branch, commit_sha, manifest_url
		FROM snapshots
		WHERE id = $1
		`,
		snapshotID,
	).Scan(
		&snapshot.Id,
		&snapshot.RepoURL,
		&snapshot.Branch,
		&snapshot.CommitSHA,
		&snapshot.ManifestURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get snapshot %q: %w", snapshotID, err)
	}

	return &snapshot, nil
}

func (r *SnapshotStoreRepo) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func (r *SnapshotStoreRepo) ensureSchema(ctx context.Context) error {
	_, err := r.pool.Exec(
		ctx,
		`
		CREATE TABLE IF NOT EXISTS snapshots (
			id TEXT PRIMARY KEY,
			repo_url TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT '',
			commit_sha TEXT NOT NULL DEFAULT '',
			manifest_url TEXT NOT NULL
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure snapshots schema: %w", err)
	}

	return nil
}

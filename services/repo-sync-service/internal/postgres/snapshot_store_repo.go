package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
	shareddomain "github.com/rohandave/tessa-rag/services/shared/domain"
	sharedpostgres "github.com/rohandave/tessa-rag/services/shared/postgres"
)

type SnapshotStoreRepo struct {
	pool *pgxpool.Pool
}

func NewSnapshotStoreRepo(ctx context.Context, cfg *sharedpostgres.DatabaseConfig) (ports.SnapshotStoreRepo, error) {
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

func (r *SnapshotStoreRepo) CreateSnapshot(snapshot *shareddomain.Snapshot) (string, error) {
	id := snapshot.Id
	if id == "" {
		id = uuid.NewString()
	}

	_, err := r.pool.Exec(
		context.Background(),
		`
		INSERT INTO snapshots (id, repo_url, branch, commit_sha, manifest_url, change_log_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		`,
		id,
		snapshot.RepoURL,
		snapshot.Branch,
		snapshot.CommitSHA,
		snapshot.ManifestURL,
		snapshot.ChangeLogURL,
	)
	if err != nil {
		return "", fmt.Errorf("insert snapshot: %w", err)
	}

	return id, nil
}

func (r *SnapshotStoreRepo) GetSnapshot(snapshotID string) (*shareddomain.Snapshot, error) {
	var snapshot shareddomain.Snapshot

	err := r.pool.QueryRow(
		context.Background(),
		`
		SELECT id, repo_url, branch, commit_sha, manifest_url, change_log_url
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
		&snapshot.ChangeLogURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get snapshot %q: %w", snapshotID, err)
	}

	return &snapshot, nil
}

func (r *SnapshotStoreRepo) GetLatestSnapshot(repoURL string) (*shareddomain.Snapshot, error) {
	var snapshot shareddomain.Snapshot

	err := r.pool.QueryRow(
		context.Background(),
		`
		SELECT id, repo_url, branch, commit_sha, manifest_url, change_log_url
		FROM snapshots
		WHERE repo_url = $1
		ORDER BY id DESC
		LIMIT 1
		`,
		repoURL,
	).Scan(
		&snapshot.Id,
		&snapshot.RepoURL,
		&snapshot.Branch,
		&snapshot.CommitSHA,
		&snapshot.ManifestURL,
		&snapshot.ChangeLogURL,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get latest snapshot for repo %q: %w", repoURL, err)
	}

	return &snapshot, nil
}

func (r *SnapshotStoreRepo) DeleteSnapshotsByRepoURL(repoURL string) error {
	_, err := r.pool.Exec(
		context.Background(),
		`
		DELETE FROM snapshots
		WHERE repo_url = $1
		`,
		repoURL,
	)
	if err != nil {
		return fmt.Errorf("delete snapshot for repo %q: %w", repoURL, err)
	}

	return nil
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
			manifest_url TEXT NOT NULL,
			change_log_url TEXT NOT NULL DEFAULT ''
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure snapshots schema: %w", err)
	}

	return nil
}

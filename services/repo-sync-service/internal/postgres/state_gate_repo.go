package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/config"
	"github.com/rohandave/tessa-rag/services/repo-sync-service/internal/sync/ports"
)

type StateGateRepo struct {
	pool *pgxpool.Pool
}

func NewStateGateRepo(ctx context.Context, cfg config.DatabaseConfig) (ports.StateGateRepo, error) {
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

	repo := &StateGateRepo{
		pool: pool,
	}

	if err := repo.ensureSchema(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return repo, nil
}

func (r *StateGateRepo) SetRepoState(repoURL string, state string) error {
	_, err := r.pool.Exec(
		context.Background(),
		`
		INSERT INTO repo_states (repo_url, state)
		VALUES ($1, $2)
		ON CONFLICT (repo_url)
		DO UPDATE SET state = EXCLUDED.state
		`,
		repoURL,
		state,
	)
	if err != nil {
		return fmt.Errorf("set repo state for %q: %w", repoURL, err)
	}

	return nil
}

func (r *StateGateRepo) GetRepoState(repoURL string) (string, error) {
	var state string

	err := r.pool.QueryRow(
		context.Background(),
		`
		SELECT state
		FROM repo_states
		WHERE repo_url = $1
		`,
		repoURL,
	).Scan(&state)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}

		return "", fmt.Errorf("get repo state for %q: %w", repoURL, err)
	}

	return state, nil
}

func (r *StateGateRepo) Close() {
	if r.pool != nil {
		r.pool.Close()
	}
}

func (r *StateGateRepo) ensureSchema(ctx context.Context) error {
	_, err := r.pool.Exec(
		ctx,
		`
		CREATE TABLE IF NOT EXISTS repo_states (
			repo_url TEXT PRIMARY KEY,
			state TEXT NOT NULL
		)
		`,
	)
	if err != nil {
		return fmt.Errorf("ensure repo_states schema: %w", err)
	}

	return nil
}

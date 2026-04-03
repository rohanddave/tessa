package postgres

import (
	"context"
	"fmt"

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

func (r *StateGateRepo) TryStartRegistration(repoURL string) (bool, error) {
	commandTag, err := r.pool.Exec(
		context.Background(),
		`
		INSERT INTO repo_states (repo_url, state)
		VALUES ($1, 'registering')
		ON CONFLICT (repo_url)
		DO UPDATE SET state = EXCLUDED.state
		WHERE repo_states.state NOT IN ('registering', 'registered', 'deleting')
		`,
		repoURL,
	)
	if err != nil {
		return false, fmt.Errorf("try start registration for %q: %w", repoURL, err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *StateGateRepo) MarkRegistered(repoURL string) error {
	_, err := r.pool.Exec(
		context.Background(),
		`
		UPDATE repo_states
		SET state = 'registered'
		WHERE repo_url = $1
		`,
		repoURL,
	)
	if err != nil {
		return fmt.Errorf("mark registered for %q: %w", repoURL, err)
	}

	return nil
}

func (r *StateGateRepo) TryStartDeletion(repoURL string) (bool, error) {
	commandTag, err := r.pool.Exec(
		context.Background(),
		`
		UPDATE repo_states
		SET state = 'deleting'
		WHERE repo_url = $1 AND state = 'registered'
		`,
		repoURL,
	)
	if err != nil {
		return false, fmt.Errorf("try start deletion for %q: %w", repoURL, err)
	}

	return commandTag.RowsAffected() > 0, nil
}

func (r *StateGateRepo) MarkDeleted(repoURL string) error {
	_, err := r.pool.Exec(
		context.Background(),
		`
		UPDATE repo_states
		SET state = 'deleted'
		WHERE repo_url = $1
		`,
		repoURL,
	)
	if err != nil {
		return fmt.Errorf("mark deleted for %q: %w", repoURL, err)
	}

	return nil
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

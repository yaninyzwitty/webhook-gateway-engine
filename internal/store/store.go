package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/repository"
)

// Store wraps sqlc Queries and the pgxpool so service layers
// can access both plain queries and transactional operations
// through a single dependency.

type Store struct {
	Queries *repository.Queries
	pool    *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{
		Queries: repository.New(pool),
		pool:    pool,
	}
}

// WithTx method runs inside a serializable transaction
// On error, the transaction will be rolled back and the error will be returned

func (s *Store) WithTx(ctx context.Context, fn func(*repository.Queries) error) error {

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.Serializable,
	})

	if err != nil {
		return fmt.Errorf("bool, beginTX error %w: ", err)
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err := fn(repository.New(tx)); err != nil {
		return fmt.Errorf("bool, fn error %w: ", err)
	}
	return tx.Commit(ctx)
}

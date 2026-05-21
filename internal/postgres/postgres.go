package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yaninyzwitty/webhook-gateway-service/internal/config"
)

type Option func(*pgxpool.Config)

func WithMaxConns(n int32) Option {
	return func(c *pgxpool.Config) {
		c.MaxConns = n
	}
}

func WithMinConns(n int32) Option {
	return func(c *pgxpool.Config) {
		c.MinConns = n
	}
}

func WithConnectFunc(fn func(context.Context, *pgx.ConnConfig) error) Option {
	return func(c *pgxpool.Config) {
		if c.BeforeConnect == nil {
			c.BeforeConnect = fn
		} else {
			originalBeforeConnect := c.BeforeConnect
			c.BeforeConnect = func(ctx context.Context, cc *pgx.ConnConfig) error {
				if err := originalBeforeConnect(ctx, cc); err != nil {
					return err
				}
				return fn(ctx, cc)
			}
		}
	}
}

func New(ctx context.Context, cfg *config.PostgresConfig, options ...Option) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse DSN: %w", err)
	}

	// apply prod defaults
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdle
	poolCfg.HealthCheckPeriod = cfg.HealthCheckPeriod

	// functional options override last (callers win)
	for _, opt := range options {
		opt(poolCfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	// ping to verify connectivity before returning a live pool
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("postgres: ping pool: %w", err)
	}
	return pool, nil
}

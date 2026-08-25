package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	poolOnce sync.Once
	pool     *pgxpool.Pool
	poolErr  error
)

// Pool returns the process-wide connection pool, opening it on first use.
//
// A pgxpool.Pool is designed to be created once per process and shared: it is
// safe for concurrent use and manages connection reuse internally. Handlers used
// to call pgxpool.New per request and Close it on the way out, which meant every
// request paid TCP connect, TLS and Postgres authentication, and left the server
// churning backends. Under concurrent load that exhausts max_connections long
// before the application is the bottleneck.
func Pool(ctx context.Context) (*pgxpool.Pool, error) {
	poolOnce.Do(func() {
		dsn := os.Getenv("DATABASE_URL")
		if dsn == "" {
			poolErr = fmt.Errorf("database: DATABASE_URL is not set")
			return
		}

		cfg, err := pgxpool.ParseConfig(dsn)
		if err != nil {
			poolErr = fmt.Errorf("database: parse DATABASE_URL: %w", err)
			return
		}

		cfg.MaxConns = envInt("DB_MAX_CONNS", 25)
		cfg.MinConns = envInt("DB_MIN_CONNS", 2)
		// Recycle connections periodically so a long-lived process does not pin
		// backends that Postgres would rather reclaim.
		cfg.MaxConnLifetime = 30 * time.Minute
		cfg.MaxConnIdleTime = 5 * time.Minute
		// Fail fast rather than queueing forever when the pool is saturated: a
		// load test should surface saturation as an error, not as unbounded
		// latency.
		cfg.ConnConfig.ConnectTimeout = 5 * time.Second

		pool, poolErr = pgxpool.NewWithConfig(ctx, cfg)
	})
	return pool, poolErr
}

// Close shuts the pool down. Only main should call this, on exit.
func Close() {
	if pool != nil {
		pool.Close()
	}
}

// Ping verifies the pool can reach the database. Used by health checks.
func Ping(ctx context.Context) error {
	p, err := Pool(ctx)
	if err != nil {
		return err
	}
	return p.Ping(ctx)
}

func envInt(key string, fallback int32) int32 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return int32(n)
}

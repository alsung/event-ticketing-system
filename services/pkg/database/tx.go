package database

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// maxTxAttempts caps how many times a serialization failure is retried.
const maxTxAttempts = 3

// InTx runs fn inside a transaction, committing if it returns nil and rolling
// back otherwise.
//
// The service layer owns the transaction and stores receive it, so one commit
// can span several stores -- tickets, payments, idempotency keys and the outbox
// all land atomically.
func InTx(ctx context.Context, opts pgx.TxOptions, fn func(pgx.Tx) error) error {
	p, err := Pool(ctx)
	if err != nil {
		return err
	}

	tx, err := p.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	if err := fn(tx); err != nil {
		// Rollback failure is not surfaced: fn's error is the interesting one,
		// and a rollback on an already-closed transaction is expected.
		_ = tx.Rollback(ctx)
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}

// InSerializableTx runs fn at SERIALIZABLE isolation, retrying when Postgres
// aborts the transaction to preserve serializability.
//
// SERIALIZABLE does not block conflicting transactions; it lets them run and
// aborts one at commit time if the interleaving could not have happened in some
// serial order. That makes retrying the caller's responsibility. Two SQLSTATEs
// are retryable:
//
//	40001 serialization_failure
//	40P01 deadlock_detected
//
// fn must therefore be safe to run more than once -- it must not mutate state
// outside the transaction, such as charging a card, before the commit succeeds.
func InSerializableTx(ctx context.Context, fn func(pgx.Tx) error) error {
	opts := pgx.TxOptions{IsoLevel: pgx.Serializable}

	var lastErr error
	for attempt := 1; attempt <= maxTxAttempts; attempt++ {
		lastErr = InTx(ctx, opts, fn)
		if lastErr == nil {
			return nil
		}
		if !isRetryable(lastErr) {
			return lastErr
		}
		if attempt < maxTxAttempts {
			sleepBackoff(ctx, attempt)
		}
	}
	return fmt.Errorf("transaction failed after %d attempts: %w", maxTxAttempts, lastErr)
}

// isRetryable reports whether Postgres aborted the transaction for a reason that
// a retry could resolve.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == "40001" || pgErr.Code == "40P01"
}

// sleepBackoff waits with exponential backoff plus jitter. Jitter matters here:
// without it, transactions that collided once retry in lockstep and collide
// again.
func sleepBackoff(ctx context.Context, attempt int) {
	base := time.Duration(1<<uint(attempt-1)) * 10 * time.Millisecond
	jitter := time.Duration(rand.Int63n(int64(base)))
	select {
	case <-time.After(base + jitter):
	case <-ctx.Done():
	}
}

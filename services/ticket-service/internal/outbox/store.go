package outbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Record is an unpublished outbox row.
type Record struct {
	ID          int64
	AggregateID uuid.UUID
	Topic       string
	Payload     []byte
	Attempts    int
}

// Enqueue writes an event inside the caller's transaction.
//
// Taking a pgx.Tx rather than a pool is the whole point: the event and the state
// change it describes must commit or roll back together. A version of this that
// took a pool would be a dual write with extra steps.
func Enqueue(ctx context.Context, tx pgx.Tx, topic string, aggregateID uuid.UUID, event any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (aggregate_id, topic, payload)
		VALUES ($1, $2, $3)
	`, aggregateID, topic, payload)
	return err
}

// FetchUnpublished claims a batch for publishing.
//
// FOR UPDATE SKIP LOCKED for the same reason the ticket claim uses it: more than
// one relay may run, and each should take a different batch rather than queue
// behind the same rows.
func FetchUnpublished(ctx context.Context, tx pgx.Tx, limit int) ([]Record, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, aggregate_id, topic, payload, attempts
		  FROM outbox
		 WHERE published_at IS NULL
		 ORDER BY id
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Record
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.AggregateID, &r.Topic, &r.Payload, &r.Attempts); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkPublished records that an event reached the broker.
func MarkPublished(ctx context.Context, tx pgx.Tx, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		UPDATE outbox SET published_at = NOW() WHERE id = ANY($1)
	`, ids)
	return err
}

// MarkFailed records a publish failure so a permanently stuck event is visible
// rather than retried silently forever.
func MarkFailed(ctx context.Context, db *pgxpool.Pool, ids []int64, reason string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := db.Exec(ctx, `
		UPDATE outbox
		   SET attempts = attempts + 1, last_error = $2
		 WHERE id = ANY($1)
	`, ids, reason)
	return err
}

// PendingCount reports how many events are waiting. Used by tests and by the
// readiness probe to show the relay is keeping up.
func PendingCount(ctx context.Context, db *pgxpool.Pool) (int, error) {
	var n int
	err := db.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&n)
	return n, err
}

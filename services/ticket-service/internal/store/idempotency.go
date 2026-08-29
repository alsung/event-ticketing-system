package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// uniqueViolation is the SQLSTATE Postgres returns for a breached unique
// constraint -- here, a key that already exists.
const uniqueViolation = "23505"

// IdempotencyRecord is a previously seen request.
type IdempotencyRecord struct {
	Key            string
	RequestHash    string
	ResponseStatus *int
	ResponseBody   []byte
	CompletedAt    *time.Time
}

// Completed reports whether the original request finished. A record that exists
// but is not complete means another attempt is in flight right now.
func (r *IdempotencyRecord) Completed() bool {
	return r.ResponseStatus != nil
}

// HashRequest fingerprints a request body so a key reused with different content
// can be detected rather than silently served the wrong stored response.
func HashRequest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// ErrKeyExists signals that the key was already reserved; the returned record
// says whether the original request completed.
var ErrKeyExists = errors.New("idempotency key already exists")

// ReserveKey claims a key for this request.
//
// Returns nil when the caller owns the key and should do the work. Returns
// ErrKeyExists along with the existing record when someone got there first --
// the caller then decides between replaying a stored response, reporting an
// in-flight conflict, or rejecting a mismatched body.
//
// The insert runs on the pool rather than inside the handler's transaction, on
// purpose: the reservation must be visible to concurrent requests immediately,
// and a row written inside an uncommitted transaction is not.
func ReserveKey(ctx context.Context, db *pgxpool.Pool, key string, userID uuid.UUID, endpoint, hash string) (*IdempotencyRecord, error) {
	_, err := db.Exec(ctx, `
		INSERT INTO idempotency_keys (key, user_id, endpoint, request_hash)
		VALUES ($1, $2, $3, $4)
	`, key, userID, endpoint, hash)

	if err == nil {
		return nil, nil
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		return nil, err
	}

	existing, err := LoadKey(ctx, db, key, userID)
	if err != nil {
		return nil, err
	}
	return existing, ErrKeyExists
}

// LoadKey reads a key belonging to a caller. Scoping by user_id means one
// caller cannot probe or collide with another's keys.
func LoadKey(ctx context.Context, db *pgxpool.Pool, key string, userID uuid.UUID) (*IdempotencyRecord, error) {
	var rec IdempotencyRecord
	err := db.QueryRow(ctx, `
		SELECT key, request_hash, response_status, response_body, completed_at
		FROM idempotency_keys
		WHERE key = $1 AND user_id = $2
	`, key, userID).Scan(
		&rec.Key, &rec.RequestHash, &rec.ResponseStatus, &rec.ResponseBody, &rec.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &rec, nil
}

// CompleteKey stores the response so a later retry replays it verbatim.
func CompleteKey(ctx context.Context, db *pgxpool.Pool, key string, status int, body []byte) error {
	_, err := db.Exec(ctx, `
		UPDATE idempotency_keys
		SET response_status = $1, response_body = $2, completed_at = NOW()
		WHERE key = $3
	`, status, body, key)
	return err
}

// ReleaseKey removes a reservation whose work failed, so the caller can retry
// rather than being locked out by their own failed attempt.
//
// Only safe for failures that changed nothing. A failure that may have charged
// a card must keep its key, so the retry is recognised as a replay.
func ReleaseKey(ctx context.Context, db *pgxpool.Pool, key string) error {
	_, err := db.Exec(ctx, `
		DELETE FROM idempotency_keys WHERE key = $1 AND response_status IS NULL
	`, key)
	return err
}

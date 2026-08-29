package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/store"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// These are integration tests: the behaviour worth testing here lives in the
// SQL and in Postgres constraints, not in Go, so a fake database would test
// nothing real. They skip unless TEST_DATABASE_URL points at a live database.
//
//	TEST_DATABASE_URL=postgres://admin:password@localhost:5433/event_ticketing?sslmode=disable go test ./internal/store/
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// fixture creates a throwaway user, event and ticket, and removes them after.
func fixture(t *testing.T, pool *pgxpool.Pool) (userID, ticketID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	email := "store-test-" + uuid.NewString() + "@example.com"
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, password_hash, full_name) VALUES ($1, '', 'Store Test') RETURNING id`,
		email).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}

	var eventID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
		VALUES ('Store Test', '', '', NOW() + interval '1 day', NOW() + interval '1 day 2 hours', $1)
		RETURNING id`, userID).Scan(&eventID); err != nil {
		t.Fatalf("insert event: %v", err)
	}

	if err := pool.QueryRow(ctx,
		`INSERT INTO tickets (event_id, price, status) VALUES ($1, 49.99, 'purchased') RETURNING id`,
		eventID).Scan(&ticketID); err != nil {
		t.Fatalf("insert ticket: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM payments WHERE ticket_id = $1`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM ticket_cancellation_logs WHERE ticket_id = $1`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, ticketID)
		_, _ = pool.Exec(ctx, `DELETE FROM events WHERE id = $1`, eventID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})
	return userID, ticketID
}

func insertPayment(t *testing.T, pool *pgxpool.Pool, userID, ticketID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var paymentID uuid.UUID
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	paymentID, err = store.InsertPending(ctx, tx, store.Payment{
		TicketID:    ticketID,
		UserID:      userID,
		IntentID:    "pi_test_" + uuid.NewString(),
		AmountCents: 4999,
		Currency:    "usd",
	})
	if err != nil {
		t.Fatalf("insert pending: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return paymentID
}

func TestPaymentLifecycle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	userID, ticketID := fixture(t, pool)

	paymentID := insertPayment(t, pool, userID, ticketID)

	// A pending payment is not refundable, so ByTicket must not return it.
	tx, _ := pool.Begin(ctx)
	_, err := store.ByTicket(ctx, tx, ticketID)
	_ = tx.Rollback(ctx)
	if err != store.ErrPaymentNotFound {
		t.Errorf("pending payment should not be refundable, got %v", err)
	}

	if err := store.MarkStatus(ctx, pool, paymentID, store.PaymentSucceeded, "pi_real_123"); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}

	tx, _ = pool.Begin(ctx)
	p, err := store.ByTicket(ctx, tx, ticketID)
	_ = tx.Rollback(ctx)
	if err != nil {
		t.Fatalf("by ticket: %v", err)
	}
	if p.Status != store.PaymentSucceeded {
		t.Errorf("status = %s, want succeeded", p.Status)
	}
	if p.IntentID != "pi_real_123" {
		t.Errorf("intent id = %s, want the real one to replace the provisional", p.IntentID)
	}
	if p.AmountCents != 4999 {
		t.Errorf("amount = %d, want 4999", p.AmountCents)
	}
}

// The invariant the payments table exists to enforce.
func TestRefundIsRejectedTwice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	userID, ticketID := fixture(t, pool)

	paymentID := insertPayment(t, pool, userID, ticketID)
	if err := store.MarkStatus(ctx, pool, paymentID, store.PaymentSucceeded, "pi_real_456"); err != nil {
		t.Fatalf("mark succeeded: %v", err)
	}

	refund := func(id string) error {
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		if err := store.MarkRefunded(ctx, tx, paymentID, id); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		return tx.Commit(ctx)
	}

	if err := refund("re_first"); err != nil {
		t.Fatalf("first refund: %v", err)
	}
	if err := refund("re_second"); err != store.ErrAlreadyRefunded {
		t.Errorf("second refund returned %v, want ErrAlreadyRefunded", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM payments WHERE id = $1 AND stripe_refund_id = 're_first'`,
		paymentID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected the first refund id to stand, found %d rows", count)
	}
}

func TestIdempotencyKeyReserveAndComplete(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	userID, _ := fixture(t, pool)

	key := "test-" + uuid.NewString()
	hash := store.HashRequest([]byte(`{"event_id":"abc"}`))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE key = $1`, key) })

	rec, err := store.ReserveKey(ctx, pool, key, userID, "/tickets/purchase", hash)
	if err != nil {
		t.Fatalf("first reserve should succeed, got %v (rec=%v)", err, rec)
	}

	// A second reservation must not be granted; the record says the original is
	// still in flight.
	rec, err = store.ReserveKey(ctx, pool, key, userID, "/tickets/purchase", hash)
	if err != store.ErrKeyExists {
		t.Fatalf("second reserve = %v, want ErrKeyExists", err)
	}
	if rec.Completed() {
		t.Error("record should not be complete while the first attempt is in flight")
	}

	if err := store.CompleteKey(ctx, pool, key, 200, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("complete: %v", err)
	}

	rec, err = store.LoadKey(ctx, pool, key, userID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !rec.Completed() {
		t.Fatal("record should be complete after CompleteKey")
	}
	if *rec.ResponseStatus != 200 {
		t.Errorf("status = %d, want 200", *rec.ResponseStatus)
	}
}

var _ = pgx.ErrNoRows

package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Payment states. Only 'pending' is transient; a payment left there means the
// process died between charging Stripe and recording the outcome, which is the
// case reconciliation exists to find.
const (
	PaymentPending   = "pending"
	PaymentSucceeded = "succeeded"
	PaymentFailed    = "failed"
	PaymentRefunded  = "refunded"
)

// ErrAlreadyRefunded means a refund was already recorded against this payment.
// It surfaces from the UNIQUE constraint on stripe_refund_id, so the guarantee
// holds even if the application logic above were wrong.
var ErrAlreadyRefunded = errors.New("payment already refunded")

// ErrPaymentNotFound means no payment row exists for the ticket.
var ErrPaymentNotFound = errors.New("payment not found")

// Payment is a row of the payments table.
type Payment struct {
	ID          uuid.UUID
	TicketID    uuid.UUID
	UserID      uuid.UUID
	IntentID    string
	AmountCents int64
	Currency    string
	Status      string
	RefundID    *string
	RefundedAt  *time.Time
}

// InsertPending records the intent to charge, inside the caller's transaction.
//
// Written before Stripe is called so that a crash mid-charge leaves evidence.
// A payment row with an intent id and status 'pending' is reconcilable against
// Stripe; a charge with no local row at all is not.
func InsertPending(ctx context.Context, tx pgx.Tx, p Payment) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO payments (ticket_id, user_id, stripe_payment_intent_id,
		                      amount_cents, currency, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, p.TicketID, p.UserID, p.IntentID, p.AmountCents, p.Currency, PaymentPending).Scan(&id)
	return id, err
}

// MarkStatus moves a payment to its terminal state and records the real intent
// id, which is only known once the processor has answered.
func MarkStatus(ctx context.Context, db *pgxpool.Pool, id uuid.UUID, status, intentID string) error {
	_, err := db.Exec(ctx, `
		UPDATE payments
		   SET status = $1,
		       stripe_payment_intent_id = COALESCE(NULLIF($2, ''), stripe_payment_intent_id),
		       updated_at = NOW()
		 WHERE id = $3
	`, status, intentID, id)
	return err
}

// ByTicket loads the succeeded payment for a ticket, which is what a refund
// needs. Runs inside the cancellation transaction so the read and the refund
// guard are part of the same serializable unit.
func ByTicket(ctx context.Context, tx pgx.Tx, ticketID uuid.UUID) (*Payment, error) {
	var p Payment
	err := tx.QueryRow(ctx, `
		SELECT id, ticket_id, user_id, stripe_payment_intent_id,
		       amount_cents, currency, status, stripe_refund_id, refunded_at
		  FROM payments
		 WHERE ticket_id = $1 AND status IN ('succeeded', 'refunded')
		 ORDER BY created_at DESC
		 LIMIT 1
	`, ticketID).Scan(&p.ID, &p.TicketID, &p.UserID, &p.IntentID,
		&p.AmountCents, &p.Currency, &p.Status, &p.RefundID, &p.RefundedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// MarkRefunded records a refund.
//
// The WHERE clause requires the payment to still be 'succeeded', so a second
// refund updates nothing and is reported rather than silently accepted. The
// UNIQUE index on stripe_refund_id is the backstop underneath that check.
func MarkRefunded(ctx context.Context, tx pgx.Tx, id uuid.UUID, refundID string) error {
	tag, err := tx.Exec(ctx, `
		UPDATE payments
		   SET status = $1,
		       stripe_refund_id = $2,
		       refunded_at = NOW(),
		       updated_at = NOW()
		 WHERE id = $3 AND status = $4
	`, PaymentRefunded, refundID, id, PaymentSucceeded)

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return ErrAlreadyRefunded
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrAlreadyRefunded
	}
	return nil
}

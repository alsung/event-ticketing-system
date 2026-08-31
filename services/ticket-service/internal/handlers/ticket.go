package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/alsung/event-ticketing-system/services/pkg/auth"
	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/outbox"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/payments"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/store"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/utils"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// callerID verifies the bearer token and returns the authenticated user.
//
// The gateway already verified this token, but services must not rely on that:
// compose no longer publishes their ports, yet defence in depth means a service
// still authenticates every request it serves rather than trusting its network
// position.
func callerID(r *http.Request) (uuid.UUID, error) {
	claims, err := auth.DefaultVerifier().FromRequest(r)
	if err != nil {
		return uuid.Nil, err
	}
	return claims.UserID, nil
}

// PurchaseTicket handles ticket purchasing logic
func PurchaseTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		EventID uuid.UUID `json:"event_id"`
		// Optional. Without one the fake provider path runs, which keeps the
		// stack usable for anyone without a Stripe account.
		PaymentMethodID string `json:"payment_method_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Securely get userID from JWT
	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	ctx := r.Context()

	var (
		ticketID     uuid.UUID
		qrCodeBase64 string
		priceCents   int64
		paymentID    uuid.UUID
	)

	// The idempotency key doubles as Stripe's key so a retry that reaches the
	// processor twice returns the original PaymentIntent. Falling back to the
	// ticket id keeps the call keyed even when the caller sent no header.
	idemKey := r.Header.Get("Idempotency-Key")

	// READ COMMITTED, not SERIALIZABLE. Claiming a ticket is a high-contention
	// operation on one hot table, and a row lock already enforces the invariant
	// exactly. SERIALIZABLE would produce a storm of serialization failures for
	// no additional safety.
	err = database.InTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// SKIP LOCKED is what makes this scale. Without it, concurrent buyers
		// queue behind one row lock; worse, PostgreSQL re-evaluates the WHERE
		// clause after each lock is released, so a waiter wakes up, finds its
		// candidate row now purchased, and with LIMIT 1 returns zero rows -- a
		// spurious "sold out" while inventory remains. SKIP LOCKED steps over
		// locked rows so each transaction claims a different one.
		//
		// ORDER BY id keeps the claim deterministic, which makes the load test
		// reproducible.
		err := tx.QueryRow(ctx, `
			SELECT id, (price * 100)::bigint FROM tickets
			WHERE event_id = $1 AND status = 'available'
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, req.EventID).Scan(&ticketID, &priceCents)
		if err != nil {
			return err
		}

		qrCodeBase64, err = utils.GenerateQRCodeBase64(ticketID.String())
		if err != nil {
			return fmt.Errorf("generate qr code: %w", err)
		}

		// The status predicate is repeated here on purpose. The row is already
		// locked, so it cannot change underneath us, but guarding the UPDATE
		// means a logic error upstream cannot silently resell a purchased
		// ticket.
		tag, err := tx.Exec(ctx, `
			UPDATE tickets
			SET user_id = $1,
				status = 'purchased',
				purchased_at = NOW(),
				qr_code = $2
			WHERE id = $3 AND status = 'available'
		`, userID, qrCodeBase64, ticketID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("expected to claim exactly 1 ticket, claimed %d", tag.RowsAffected())
		}

		// Record the intent to charge before calling Stripe, so a crash between
		// the two leaves evidence. A payment row holding a provisional id and
		// status 'pending' can be reconciled against Stripe; a charge with no
		// local row at all cannot. The real intent id replaces the provisional
		// one once the processor answers.
		//
		// The provisional id is unique per attempt, not per ticket: a failed
		// charge releases the seat, so the same ticket can be claimed again and
		// a ticket-derived placeholder would collide on the unique index.
		paymentID, err = store.InsertPending(ctx, tx, store.Payment{
			TicketID:    ticketID,
			UserID:      userID,
			IntentID:    "pending_" + uuid.NewString(),
			AmountCents: priceCents,
			Currency:    "usd",
		})
		if err != nil {
			return err
		}

		// The event goes into the outbox inside this transaction, not published
		// to Kafka here. Postgres and Kafka share no transaction, so producing
		// directly would be a dual write: commit-then-produce loses the event if
		// this process dies in between, and produce-then-commit announces a
		// purchase that may roll back. Writing the intent-to-publish alongside
		// the state change makes the pair atomic; a relay does the producing.
		return outbox.Enqueue(ctx, tx, outbox.TopicTicketPurchased, ticketID,
			outbox.TicketPurchased{
				MessageID:   uuid.New(),
				EventType:   outbox.TopicTicketPurchased,
				TicketID:    ticketID,
				EventID:     req.EventID,
				UserID:      userID,
				AmountCents: priceCents,
				OccurredAt:  time.Now().UTC(),
			})
	})

	if err != nil {
		// No row returned means the pool is empty. That is contention, not a
		// server fault, so it must be distinguishable from a 500 -- the load
		// test asserts on exactly this boundary.
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "No tickets available for this event", http.StatusConflict)
			return
		}
		log.Printf("purchase failed for event %s: %v", req.EventID, err)
		http.Error(w, "Failed to purchase ticket", http.StatusInternalServerError)
		return
	}

	// The ticket is claimed and the payment is recorded as pending. Charging
	// happens outside the transaction because it is a side effect on another
	// system: holding a database transaction open across a network call to
	// Stripe would pin a connection for the duration of someone else's latency,
	// and a rollback could not un-charge the card anyway.
	// Falling back to the payment id, not the ticket id. A failed charge
	// releases the seat, so the same ticket can be bought again -- and Stripe
	// rejects a key reused with different parameters. One payment row is one
	// charge attempt, which is exactly the granularity an idempotency key wants.
	if idemKey == "" {
		idemKey = "payment_" + paymentID.String()
	}
	charge, chargeErr := payments.Default().Charge(ctx, payments.ChargeRequest{
		AmountCents:     priceCents,
		Currency:        "usd",
		PaymentMethodID: req.PaymentMethodID,
		IdempotencyKey:  idemKey,
		Metadata: map[string]string{
			"ticket_id": ticketID.String(),
			"user_id":   userID.String(),
			"event_id":  req.EventID.String(),
		},
	})

	if chargeErr != nil {
		// Compensating transaction: the money did not move, so the seat must go
		// back. This is the price of doing the charge outside the transaction --
		// there is no rollback, only an explicit undo.
		releaseAfterFailedCharge(ctx, ticketID, paymentID)

		switch {
		case errors.Is(chargeErr, payments.ErrCardDeclined):
			http.Error(w, "Card declined", http.StatusPaymentRequired)
		case errors.Is(chargeErr, payments.ErrProviderUnavailable):
			log.Printf("payment provider unavailable for ticket %s: %v", ticketID, chargeErr)
			http.Error(w, "Payment provider unavailable", http.StatusBadGateway)
		default:
			log.Printf("charge failed for ticket %s: %v", ticketID, chargeErr)
			http.Error(w, "Payment failed", http.StatusBadGateway)
		}
		return
	}

	// Settling the payment row is deliberately not fatal. The customer has been
	// charged and holds the ticket; failing the request here would tell them the
	// purchase did not happen when it did. The row stays 'pending' for
	// reconciliation instead.
	if db, err := database.Pool(ctx); err == nil {
		if err := store.MarkStatus(ctx, db, paymentID, store.PaymentSucceeded, charge.ProviderID); err != nil {
			log.Printf("charge %s succeeded but marking payment %s failed: %v",
				charge.ProviderID, paymentID, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ticket_id":  ticketID,
		"message":    "Ticket successfully purchased",
		"qr_code":    qrCodeBase64,
		"payment_id": charge.ProviderID,
		"amount":     priceCents,
	})
}

// releaseAfterFailedCharge returns a claimed ticket to the pool after the
// charge failed, and marks the payment row failed.
//
// Best effort by necessity: if this fails the ticket is stranded as purchased
// with a failed payment, which is exactly what a reconciliation sweep is for.
// Logging loudly is the most this path can honestly do.
func releaseAfterFailedCharge(ctx context.Context, ticketID, paymentID uuid.UUID) {
	err := database.InTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE tickets
			   SET status = 'available', user_id = NULL, purchased_at = NULL, qr_code = NULL
			 WHERE id = $1 AND status = 'purchased'
		`, ticketID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE payments SET status = $1, updated_at = NOW() WHERE id = $2`,
			store.PaymentFailed, paymentID)
		return err
	})
	if err != nil {
		log.Printf("CRITICAL: ticket %s stranded after failed charge, payment %s: %v",
			ticketID, paymentID, err)
	}
}

// ListAvailableTickets lists available tickets for a given event
func ListAvailableTickets(w http.ResponseWriter, r *http.Request) {
	eventIDStr := r.URL.Query().Get("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(ctx, `
		SELECT id, price, created_at FROM tickets
		WHERE event_id = $1 AND status = 'available'
	`, eventID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Ticket struct {
		ID        uuid.UUID `json:"id"`
		Price     float64   `json:"price"`
		CreatedAt time.Time `json:"created_at"`
	}

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		if err := rows.Scan(&t.ID, &t.Price, &t.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tickets = append(tickets, t)
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tickets)
}

// CreateTickets allows organizers/admin to create multiple tickets for an event
func CreateTickets(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID  uuid.UUID `json:"event_id"`
		Price    float64   `json:"price"`
		Quantity int       `json:"quantity"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Quantity <= 0 {
		http.Error(w, "Invalid quantity", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Extract user ID from JWT
	userID, err := callerID(r)
	log.Println("userID", userID)
	if err != nil {
		log.Println("err", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	isAdmin, err := store.IsAdmin(ctx, db, userID)
	if err != nil {
		http.Error(w, "Error checking admin status", http.StatusInternalServerError)
		return
	}

	// Validate that the user is the organizer of the event or admin
	var organizerID uuid.UUID
	err = db.QueryRow(ctx, `
		SELECT organizer_id FROM events WHERE id = $1
	`, req.EventID).Scan(&organizerID)

	if err != nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	// Allow admin OR organizer to create tickets
	if userID != organizerID && !isAdmin {
		http.Error(w, "Forbidden: You are not the organizer or admin", http.StatusForbidden)
		return
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		http.Error(w, "Transaction error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i := 0; i < req.Quantity; i++ {
		_, err = tx.Exec(ctx, `
			INSERT INTO tickets (event_id, price)
			VALUES ($1, $2)
		`, req.EventID, req.Price)

		if err != nil {
			http.Error(w, "Failed to create tickets", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "Tickets created successfully",
		"quantity": req.Quantity,
	})
}

// GetUserTickets lists all tickets purchased by a user
func GetUserTickets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(ctx, `
		SELECT id, event_id, price, status, purchased_at, qr_code
		FROM tickets
		WHERE user_id = $1 AND status = 'purchased'
	`, userID)
	if err != nil {
		http.Error(w, "Query error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type PurchasedTicket struct {
		ID          uuid.UUID `json:"id"`
		EventID     uuid.UUID `json:"event_id"`
		Price       float64   `json:"price"`
		Status      string    `json:"status"`
		PurchasedAt time.Time `json:"purchased_at"`
		QRCode      *string   `json:"qr_code,omitempty"`
	}

	var tickets []PurchasedTicket
	for rows.Next() {
		var t PurchasedTicket
		var qr *string

		if err := rows.Scan(&t.ID, &t.EventID, &t.Price, &t.Status, &t.PurchasedAt, &qr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		t.QRCode = qr

		tickets = append(tickets, t)
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(tickets)
}

// CancelTicket allows users to cancel one of their purchased tickets
// which makes it available again for others to purchase.
// It also logs the cancellation in ticket_cancellation_logs.
func CancelTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TicketID uuid.UUID `json:"ticket_id"`
		Reason   string    `json:"reason"` // optional
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var reasonPtr *string
	if req.Reason != "" {
		reasonPtr = &req.Reason
	}

	// Sentinel errors let the transaction body signal an outcome that maps to a
	// specific status code, without the closure knowing about HTTP.
	var errNotPurchased = errors.New("ticket is not in purchased state")

	// Populated inside the transaction, used after it commits. A refund is a
	// call to another system, so it cannot happen inside the transaction for the
	// same reason the charge could not.
	var payment *store.Payment

	// SERIALIZABLE here, unlike the purchase path. Cancellation spans a
	// read-then-write across tickets and ticket_cancellation_logs, and Phase 2
	// adds a refund guard against payments. That invariant -- a ticket is
	// refunded at most once -- is exactly the shape a row lock does not protect
	// and serializable isolation does. Volume is low enough to absorb retries.
	err = database.InSerializableTx(ctx, func(tx pgx.Tx) error {
		var status string
		var eventID uuid.UUID

		// Every statement below runs on tx. Previously the SELECT and UPDATE ran
		// on the pool while only the audit insert ran on the transaction, so the
		// ticket was released outside the transaction entirely and a failed
		// insert rolled back nothing.
		err := tx.QueryRow(ctx, `
			SELECT status, event_id FROM tickets
			WHERE id = $1 AND user_id = $2
		`, req.TicketID, userID).Scan(&status, &eventID)
		if err != nil {
			return err
		}
		if status != "purchased" {
			return errNotPurchased
		}

		tag, err := tx.Exec(ctx, `
			UPDATE tickets
			SET status = 'available',
				user_id = NULL,
				purchased_at = NULL,
				qr_code = NULL
			WHERE id = $1 AND status = 'purchased'
		`, req.TicketID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			// Another transaction cancelled it first. Serializable isolation
			// would abort one of the two, but guarding the predicate makes the
			// intent explicit.
			return errNotPurchased
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO ticket_cancellation_logs (ticket_id, user_id, event_id, reason)
			VALUES ($1, $2, $3, $4)
		`, req.TicketID, userID, eventID, reasonPtr); err != nil {
			return err
		}

		if err := outbox.Enqueue(ctx, tx, outbox.TopicTicketCancelled, req.TicketID,
			outbox.TicketCancelled{
				MessageID:  uuid.New(),
				EventType:  outbox.TopicTicketCancelled,
				TicketID:   req.TicketID,
				EventID:    eventID,
				UserID:     userID,
				Reason:     req.Reason,
				OccurredAt: time.Now().UTC(),
			}); err != nil {
			return err
		}

		// Read the payment inside the transaction so that the
		// refunded-at-most-once check sees a consistent snapshot. This is the
		// invariant that makes cancellation worth running at SERIALIZABLE: it
		// spans tickets, ticket_cancellation_logs and payments.
		p, err := store.ByTicket(ctx, tx, req.TicketID)
		switch {
		case errors.Is(err, store.ErrPaymentNotFound):
			// A ticket bought before payments existed, or one seeded directly.
			// Cancelling is still correct; there is simply nothing to refund.
			return nil
		case err != nil:
			return err
		case p.Status == store.PaymentRefunded:
			return nil
		}
		payment = p
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			http.Error(w, "Ticket not found or not owned by caller", http.StatusNotFound)
		case errors.Is(err, errNotPurchased):
			http.Error(w, "Only purchased tickets can be cancelled", http.StatusConflict)
		default:
			log.Printf("cancel failed for ticket %s: %v", req.TicketID, err)
			http.Error(w, "Failed to cancel ticket", http.StatusInternalServerError)
		}
		return
	}

	resp := map[string]interface{}{
		"message": "Ticket cancelled and returned to pool",
	}

	// The seat is already back in the pool. A refund that fails here must not
	// fail the request: the customer would be told the cancellation did not
	// happen when it did, and would likely retry, cancelling nothing and still
	// being owed money. The payment stays 'succeeded' for a reconciliation
	// sweep to pick up.
	if payment != nil {
		refund, err := payments.Default().Refund(ctx, payments.RefundRequest{
			ChargeID:       payment.IntentID,
			AmountCents:    payment.AmountCents,
			IdempotencyKey: "refund_" + payment.ID.String(),
			Metadata:       map[string]string{"ticket_id": req.TicketID.String()},
		})
		if err != nil {
			log.Printf("ticket %s cancelled but refund of payment %s failed: %v",
				req.TicketID, payment.ID, err)
			resp["refund_status"] = "pending"
		} else {
			markRefunded(ctx, payment.ID, refund.ProviderID)
			resp["refund_id"] = refund.ProviderID
			resp["refund_status"] = "refunded"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// markRefunded records a completed refund. The UNIQUE constraint on
// stripe_refund_id is the real guarantee that one payment is refunded once;
// this write is how that guarantee gets exercised.
func markRefunded(ctx context.Context, paymentID uuid.UUID, refundID string) {
	err := database.InTx(ctx, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return store.MarkRefunded(ctx, tx, paymentID, refundID)
	})
	if err != nil {
		log.Printf("refund %s issued but marking payment %s failed: %v", refundID, paymentID, err)
	}
}

// GetTicketReceipt returns the ticket info along with the QR code
func GetTicketReceipt(w http.ResponseWriter, r *http.Request) {
	log.Println("GetTicketReceipt called")
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ticketIDStr := r.URL.Query().Get("ticket_id")
	if ticketIDStr == "" {
		http.Error(w, "Missing ticket_id", http.StatusBadRequest)
		return
	}

	ticketID, err := uuid.Parse(ticketIDStr)
	if err != nil {
		http.Error(w, "Invalid ticket_id format", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	var (
		eventID     uuid.UUID
		price       float64
		status      string
		purchasedAt time.Time
		qrCode      string
	)

	err = db.QueryRow(ctx, `
		SELECT event_id, price, status, purchased_at, qr_code
		FROM tickets
		WHERE id = $1 AND user_id = $2
	`, ticketID, userID).Scan(&eventID, &price, &status, &purchasedAt, &qrCode)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Ticket not found or not owned by user", http.StatusNotFound)
		} else {
			http.Error(w, "Query error", http.StatusInternalServerError)
		}
		return
	}

	resp := map[string]interface{}{
		"ticket_id":    ticketID,
		"event_id":     eventID,
		"price":        price,
		"status":       status,
		"purchased_at": purchasedAt,
		"qr_code":      qrCode,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

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
	)

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
			SELECT id FROM tickets
			WHERE event_id = $1 AND status = 'available'
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		`, req.EventID).Scan(&ticketID)
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
		return nil
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ticket_id": ticketID,
		"message":   "Ticket successfully purchased",
		"qr_code":   qrCodeBase64,
	})
}

// ListAvailableTickets lists available tickets for a given event
func ListAvailableTickets(w http.ResponseWriter, r *http.Request) {
	eventIDStr := r.URL.Query().Get("event_id")
	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		http.Error(w, "Invalid event ID", http.StatusBadRequest)
		return
	}

	db, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	rows, err := db.Query(context.Background(), `
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

	ctx := context.Background()

	// Extract user ID from JWT
	userID, err := callerID(r)
	log.Println("userID", userID)
	if err != nil {
		log.Println("err", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.NewDatabaseConnection(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

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

	ctx := context.Background()

	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.NewDatabaseConnection(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

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

		_, err = tx.Exec(ctx, `
			INSERT INTO ticket_cancellation_logs (ticket_id, user_id, event_id, reason)
			VALUES ($1, $2, $3, $4)
		`, req.TicketID, userID, eventID, reasonPtr)
		return err
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Ticket cancelled and returned to pool",
	})
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

	ctx := context.Background()
	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.NewDatabaseConnection(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

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

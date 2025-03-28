package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/google/uuid"
)

// PurchaseTicket handles ticket purchasing logic
func PurchaseTicket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID uuid.UUID `json:"event_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Securely get userID from JWT
	userID, err := middleware.GetUserIDFromJWT(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	db, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

	tx, err := db.Begin(context.Background())
	if err != nil {
		http.Error(w, "Transaction error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	var ticketID uuid.UUID
	err = tx.QueryRow(context.Background(), `
		SELECT id FROM tickets
		WHERE event_id = $1 AND status = 'available'
		LIMIT 1 FOR UPDATE
	`, req.EventID).Scan(&ticketID)

	if err != nil {
		http.Error(w, "No available tickets", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(context.Background(), `
		UPDATE tickets
		SET status = 'purchased', user_id = $1, purchased_at = NOW()
		WHERE id = $2
	`, userID, ticketID)

	if err != nil {
		http.Error(w, "Failed to update ticket", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(context.Background()); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ticket_id": ticketID,
		"message":   "Ticket successfully purchased",
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
	json.NewEncoder(w).Encode(tickets)
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
	userID, err := middleware.GetUserIDFromJWT(r)
	log.Println("userID", userID)
	if err != nil {
		log.Println("err", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isAdmin, err := middleware.IsAdmin(ctx, userID)
	if err != nil {
		http.Error(w, "Error checking admin status", http.StatusInternalServerError)
		return
	}

	db, err := database.NewDatabaseConnection(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close()

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
	defer tx.Rollback(ctx)

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
	json.NewEncoder(w).Encode(map[string]interface{}{
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

	userID, err := middleware.GetUserIDFromJWT(r)
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
		var qr sql.NullString

		if err := rows.Scan(&t.ID, &t.EventID, &t.Price, &t.Status, &t.PurchasedAt, &qr); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if qr.Valid {
			t.QRCode = &qr.String
		}

		tickets = append(tickets, t)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tickets)
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

	ctx := context.Background()
	userID, err := middleware.GetUserIDFromJWT(r)
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

	tx, err := db.Begin(ctx)
	if err != nil {
		http.Error(w, "Transaction begin failed", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(ctx)

	// Ensure ticket belongs to user and is in 'purchased' status
	var existingStatus string
	var eventID uuid.UUID
	err = db.QueryRow(ctx, `
		SELECT status, event_id FROM tickets
		WHERE id = $1 AND user_id = $2
	`, req.TicketID, userID).Scan(&existingStatus, &eventID)

	if err != nil {
		http.Error(w, "Ticket not found", http.StatusNotFound)
		return
	}

	if existingStatus != "purchased" {
		http.Error(w, "Only purchased tickets can be cancelled", http.StatusBadRequest)
		return
	}

	// Set ticket status back to 'available' and null out user-specific info
	_, err = db.Exec(ctx, `
		UPDATE tickets
		SET status = 'available',
			user_id = NULL,
			purchased_at = NULL,
			qr_code = NULL
		WHERE id = $1
	`, req.TicketID)
	if err != nil {
		http.Error(w, "Failed to cancel ticket", http.StatusInternalServerError)
		return
	}

	// Log the cancellation
	var reasonPtr *string
	if req.Reason != "" {
		reasonPtr = &req.Reason
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO ticket_cancellation_logs (ticket_id, user_id, event_id, reason)
		VALUES ($1, $2, $3, $4)
	`, req.TicketID, userID, eventID, reasonPtr)
	if err != nil {
		http.Error(w, "Failed to log ticket cancellation", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Ticket cancelled and returned to pool",
	})
}

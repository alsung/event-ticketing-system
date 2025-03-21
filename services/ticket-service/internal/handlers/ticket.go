package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/database"
	"github.com/google/uuid"
)

// PurchaseTicket handles ticket purchasing logic
func PurchaseTicket(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID uuid.UUID `json:"event_id"`
		UserID  uuid.UUID `json:"user_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	db, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close(context.Background())

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
		http.Error(w, "Could not purchase ticket", http.StatusInternalServerError)
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
	defer db.Close(context.Background())

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
		CreatedAt string    `json:"created_at"`
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

	// Extract user ID from JWT
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
	defer db.Close(context.Background())

	// Validate that the user is the organizer of the event
	var organizerID uuid.UUID
	err = db.QueryRow(context.Background(), `
		SELECT organizer_id FROM events WHERE id = $1
	`, req.EventID).Scan(&organizerID)

	if err != nil {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}

	if userID != organizerID {
		http.Error(w, "Forbidden: You are not the organizer/admin", http.StatusForbidden)
		return
	}

	tx, err := db.Begin(context.Background())
	if err != nil {
		http.Error(w, "Transaction error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(context.Background())

	for i := 0; i < req.Quantity; i++ {
		_, err = tx.Exec(context.Background(), `
			INSERT INTO tickets (event_id, price)
			VALUES ($1, $2)
		`, req.EventID, req.Price)

		if err != nil {
			http.Error(w, "Failed to create tickets", http.StatusInternalServerError)
			return
		}
	}

	if err := tx.Commit(context.Background()); err != nil {
		http.Error(w, "Transaction commit failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "Tickets created successfully",
		"quantity": req.Quantity,
	})
}

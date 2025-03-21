package handlers

import (
	"context"
	"encoding/json"
	"net/http"

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

	json.NewEncoder(w).Encode(tickets)
}

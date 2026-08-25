package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/event-service/internal/models"
	"github.com/alsung/event-ticketing-system/services/pkg/database"
)

// CreateEvent handles creating new events
func CreateEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var event models.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	_, err = db.Exec(ctx,
		`INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		event.Name, event.Description, event.Location, event.StartTime, event.EndTime, event.OrganizerID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"message": "Event created successfully",
	})
}

// GetEvents retrieves a list of all events
func GetEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	rows, err := db.Query(ctx,
		"SELECT id, name, description, location, start_time, end_time, organizer_id, created_at FROM events")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		if err := rows.Scan(&event.ID, &event.Name, &event.Description, &event.Location, &event.StartTime, &event.EndTime, &event.OrganizerID, &event.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events = append(events, event)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

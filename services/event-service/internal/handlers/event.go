package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/alsung/event-ticketing-system/services/event-service/internal/models"
	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// GetEvent returns a single event by id.
//
// Public, like the listing: browsing an event is not privileged. The detail page
// needs it because filtering the full list client-side stops working the moment
// that list is paginated.
func GetEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Go 1.22+ path wildcards. "/events/create" is the more specific pattern, so
	// ServeMux still routes that to CreateEvent rather than matching it here.
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid event id", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	var event models.Event
	err = db.QueryRow(ctx, `
		SELECT id, name, description, location, start_time, end_time, organizer_id, created_at
		  FROM events
		 WHERE id = $1
	`, id).Scan(&event.ID, &event.Name, &event.Description, &event.Location,
		&event.StartTime, &event.EndTime, &event.OrganizerID, &event.CreatedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Event not found", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("get event %s: %v", id, err)
		http.Error(w, "Could not load event", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(event)
}

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

// GetEvents lists events, optionally filtered by a full-text query.
//
// Search runs in Postgres rather than a dedicated engine. At this scale an
// external index would add a synchronisation problem without solving one that
// exists; the README records the threshold where that trade flips.
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

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var rows pgx.Rows
	if query == "" {
		rows, err = db.Query(ctx, `
			SELECT id, name, description, location, start_time, end_time, organizer_id, created_at
			  FROM events
			 ORDER BY start_time
		`)
	} else {
		// websearch_to_tsquery, not plainto_tsquery: it accepts what people
		// actually type -- quoted phrases, OR, and a leading minus to exclude --
		// and it never errors on malformed input, so a stray operator returns no
		// results rather than a 500.
		//
		// ts_rank_cd weighs term proximity as well as frequency, so an event
		// whose name contains both words outranks one that mentions them apart.
		rows, err = db.Query(ctx, `
			SELECT id, name, description, location, start_time, end_time, organizer_id, created_at
			  FROM events
			 WHERE search_vector @@ websearch_to_tsquery('english', $1)
			 ORDER BY ts_rank_cd(search_vector, websearch_to_tsquery('english', $1)) DESC,
			          start_time
		`, query)
	}

	if err != nil {
		log.Printf("list events (q=%q): %v", query, err)
		http.Error(w, "Could not load events", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	events := []models.Event{}
	for rows.Next() {
		var event models.Event
		if err := rows.Scan(&event.ID, &event.Name, &event.Description, &event.Location,
			&event.StartTime, &event.EndTime, &event.OrganizerID, &event.CreatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

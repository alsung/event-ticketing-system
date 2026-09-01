package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/alsung/event-ticketing-system/services/pkg/auth"
	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Roles. Kept in step with the users.role check constraint.
const (
	roleOrganizer = "organizer"
	roleAdmin     = "admin"
)

// callerID verifies the bearer token and returns the authenticated user.
//
// The gateway already verified this token, but a service must not trust its
// network position: it authenticates every request it serves.
func callerID(r *http.Request) (uuid.UUID, error) {
	claims, err := auth.DefaultVerifier().FromRequest(r)
	if err != nil {
		return uuid.Nil, err
	}
	return claims.UserID, nil
}

// OrganizerEvent is one row of an organiser's own listing, with the sales
// figures that make the page worth loading.
type OrganizerEvent struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	Location    *string   `json:"location"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	Total       int       `json:"total_tickets"`
	Sold        int       `json:"sold_tickets"`
	Available   int       `json:"available_tickets"`
	Revenue     float64   `json:"revenue"`
}

// MyEvents lists the events the caller organises.
//
// Admins see everything; organisers see only their own. Registered as a more
// specific pattern than /events/{id}, so ServeMux routes it here rather than
// treating "mine" as an id.
func MyEvents(w http.ResponseWriter, r *http.Request) {
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

	role, err := userRole(r, userID)
	if err != nil {
		http.Error(w, "Could not check permissions", http.StatusInternalServerError)
		return
	}
	if role != roleOrganizer && role != roleAdmin {
		http.Error(w, "You do not have an organiser account", http.StatusForbidden)
		return
	}

	// Aggregating in SQL rather than fetching tickets and counting in Go: the
	// database already has the rows, and shipping a few hundred per event to
	// count them is work nobody needs done twice.
	rows, err := db.Query(ctx, `
		SELECT e.id, e.name, e.description, e.location, e.start_time, e.end_time,
		       count(t.id)                                                        AS total,
		       count(t.id) FILTER (WHERE t.status = 'purchased')                   AS sold,
		       count(t.id) FILTER (WHERE t.status = 'available')                   AS available,
		       COALESCE(sum(t.price) FILTER (WHERE t.status = 'purchased'), 0)     AS revenue
		  FROM events e
		  LEFT JOIN tickets t ON t.event_id = e.id
		 WHERE $2 = 'admin' OR e.organizer_id = $1
		 GROUP BY e.id
		 ORDER BY e.start_time
	`, userID, role)
	if err != nil {
		log.Printf("my events for %s: %v", userID, err)
		http.Error(w, "Could not load your events", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	events := []OrganizerEvent{}
	for rows.Next() {
		var e OrganizerEvent
		if err := rows.Scan(&e.ID, &e.Name, &e.Description, &e.Location,
			&e.StartTime, &e.EndTime, &e.Total, &e.Sold, &e.Available, &e.Revenue); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

// UpdateEvent edits an event the caller owns.
func UpdateEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "Invalid event id", http.StatusBadRequest)
		return
	}

	userID, err := callerID(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Location    string `json:"location"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		http.Error(w, "An event needs a name", http.StatusBadRequest)
		return
	}

	db, err := database.Pool(ctx)
	if err != nil {
		http.Error(w, "Database unavailable", http.StatusInternalServerError)
		return
	}

	role, err := userRole(r, userID)
	if err != nil {
		http.Error(w, "Could not check permissions", http.StatusInternalServerError)
		return
	}

	// Ownership lives in the WHERE clause, not in a separate read-then-write.
	// Checking first and updating second leaves a window where ownership could
	// change between the two.
	tag, err := db.Exec(ctx, `
		UPDATE events
		   SET name = $1, description = $2, location = $3, start_time = $4, end_time = $5
		 WHERE id = $6
		   AND ($8 = 'admin' OR organizer_id = $7)
	`, req.Name, req.Description, req.Location, req.StartTime, req.EndTime, id, userID, role)
	if err != nil {
		log.Printf("update event %s: %v", id, err)
		http.Error(w, "Could not update the event", http.StatusInternalServerError)
		return
	}

	if tag.RowsAffected() == 0 {
		// Deliberately not distinguishing "does not exist" from "not yours":
		// telling a stranger which event ids are real is an enumeration gift.
		http.Error(w, "Event not found, or not yours to edit", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Event updated"})
}

// userRole reads the caller's role.
func userRole(r *http.Request, userID uuid.UUID) (string, error) {
	db, err := database.Pool(r.Context())
	if err != nil {
		return "", err
	}
	var role string
	err = db.QueryRow(r.Context(), `SELECT role FROM users WHERE id = $1`, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("no such user")
	}
	return role, err
}

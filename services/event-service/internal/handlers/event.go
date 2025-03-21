package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/event-service/internal/database"
	"github.com/alsung/event-ticketing-system/services/event-service/internal/models"
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

	db, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close(context.Background())

	_, err = db.Exec(context.Background(),
		`INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		event.Name, event.Description, event.Location, event.StartTime, event.EndTime, event.OrganizerID)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Event created successfully",
	})
}

// GetEvents retrieves a list of all events
func GetEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	db, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer db.Close(context.Background())

	rows, err := db.Query(context.Background(),
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
	json.NewEncoder(w).Encode(events)
}

// // CreateEvent creates a new event
// func CreateEvent(c *gin.Context) {
// 	var event models.Event
// 	if err := c.ShouldBindJSON(&event); err != nil {
// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 		return
// 	}

// 	conn, err := database.NewDatabaseConnection(context.Background())
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	defer conn.Close(context.Background())

// 	_, err = conn.Exec(context.Background(),
// 		`INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
// 		VALUES ($1, $2, $3, $4, $5, $6)`,
// 		event.Name, event.Description, event.Location, event.StartTime, event.EndTime, event.OrganizerID)

// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}

// 	c.JSON(http.StatusCreated, gin.H{"message": "Event created successfully"})
// }

// // GetEvents lists all events
// func GetEvents(c *gin.Context) {
// 	conn, err := database.NewDatabaseConnection(context.Background())
// 	if err != nil {
// 		c.JSON((http.StatusInternalServerError), gin.H{"error": err.Error()})
// 		return
// 	}
// 	defer conn.Close(context.Background())

// 	rows, err := conn.Query(context.Background(),
// 		"SELECT id, name, description, location, start_time, end_time, organizer_id, created_at FROM events")
// 	if err != nil {
// 		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 		return
// 	}
// 	defer rows.Close()

// 	var events []models.Event
// 	for rows.Next() {
// 		var event models.Event
// 		if err := rows.Scan(&event.ID, &event.Name, &event.Description, &event.Location, &event.StartTime, &event.EndTime, &event.OrganizerID, &event.CreatedAt); err != nil {
// 			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
// 			return
// 		}
// 		events = append(events, event)
// 	}

// 	c.JSON(http.StatusOK, events)
// }

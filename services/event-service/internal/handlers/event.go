package handlers

import (
	"context"
	"net/http"

	"github.com/alsung/event-ticketing-system/event-service/internal/database"
	"github.com/alsung/event-ticketing-system/event-service/internal/models"
	"github.com/gin-gonic/gin"
)

// CreateEvent creates a new event
func CreateEvent(c *gin.Context) {
	var event models.Event
	if err := c.ShouldBindJSON(&event); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conn, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(context.Background(),
		`INSERT INTO events (name, description, location, start_time, end_time, organizer_id)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		event.Name, event.Description, event.Location, event.StartTime, event.EndTime, event.OrganizerID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "Event created successfully"})
}

// GetEvents lists all events
func GetEvents(c *gin.Context) {
	conn, err := database.NewDatabaseConnection(context.Background())
	if err != nil {
		c.JSON((http.StatusInternalServerError), gin.H{"error": err.Error()})
		return
	}
	defer conn.Close(context.Background())

	rows, err := conn.Query(context.Background(),
		"SELECT id, name, description, location, start_time, end_time, organizer_id, created_at FROM events")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var events []models.Event
	for rows.Next() {
		var event models.Event
		if err := rows.Scan(&event.ID, &event.Name, &event.Description, &event.Location, &event.StartTime, &event.EndTime, &event.OrganizerID, &event.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		events = append(events, event)
	}

	c.JSON(http.StatusOK, events)
}

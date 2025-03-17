package models

import "time"

type Event struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	OrganizerID string    `json:"organizer_id"`
	CreatedAt   time.Time `json:"created_at"`
}

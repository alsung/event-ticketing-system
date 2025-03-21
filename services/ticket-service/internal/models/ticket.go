package models

import (
	"time"

	"github.com/google/uuid"
)

type Ticket struct {
	ID          uuid.UUID  `json:"id"`
	EventID     uuid.UUID  `json:"event_id"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	Price       float64    `json:"price"`
	Status      string     `json:"status"`
	PurchasedAt *time.Time `json:"purchased_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

package models

import (
	"time"

	"github.com/google/uuid"
)

type Ticket struct {
	ID          uuid.UUID  `json:"id"`
	EventID     uuid.UUID  `json:"event_id"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	QRCode      *string    `json:"qr_code,omitempty"`
	Status      string     `json:"status"`
	Price       float64    `json:"price"`
	PurchasedAt *time.Time `json:"purchased_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

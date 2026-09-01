package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Roles. A role says who someone is; ownership decides what they may touch.
const (
	RoleAttendee  = "attendee"
	RoleOrganizer = "organizer"
	RoleAdmin     = "admin"
)

// Role returns a user's role.
//
// This lived in pkg/middleware, which meant every service importing the shared
// logging middleware also compiled in a Postgres driver -- including the API
// gateway, which has no database. Reading a user's role is a domain query
// against this service's data, not a cross-cutting HTTP concern.
func Role(ctx context.Context, db *pgxpool.Pool, userID uuid.UUID) (string, error) {
	var role string
	err := db.QueryRow(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role)
	if err != nil {
		return "", err
	}
	return role, nil
}

// MayMintTickets reports whether a user can create inventory for an event.
//
// Admins may mint for anything. An organiser may mint only for events they own,
// which is an ownership check rather than a privilege one -- the reason a role
// boolean was not enough.
func MayMintTickets(ctx context.Context, db *pgxpool.Pool, userID, eventID uuid.UUID) (bool, error) {
	var allowed bool
	err := db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM users u
			  LEFT JOIN events e ON e.id = $2
			 WHERE u.id = $1
			   AND (u.role = 'admin' OR (u.role = 'organizer' AND e.organizer_id = u.id))
		)
	`, userID, eventID).Scan(&allowed)
	return allowed, err
}

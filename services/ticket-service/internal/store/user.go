package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IsAdmin reports whether a user carries the admin flag.
//
// This lived in pkg/middleware, which meant every service importing the shared
// logging middleware also compiled in a Postgres driver -- including the API
// gateway, which has no database. Checking a user's role is a domain query
// against this service's data, not a cross-cutting HTTP concern, so it belongs
// here.
func IsAdmin(ctx context.Context, db *pgxpool.Pool, userID uuid.UUID) (bool, error) {
	var isAdmin bool
	err := db.QueryRow(ctx, `SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
	if err != nil {
		return false, err
	}
	return isAdmin, nil
}

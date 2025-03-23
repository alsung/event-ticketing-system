// services/pkg/database/connection.go
package database

import (
	"context"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func NewDatabaseConnection(ctx context.Context) (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	return pgxpool.New(ctx, dbURL)
}

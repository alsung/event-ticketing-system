package middleware

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
)

// GetUserFromJWT extracts UserID from JWT
func GetUserIDFromJWT(r *http.Request) (uuid.UUID, error) {
	tokenStr := r.Header.Get("Authorization")
	if tokenStr == "" {
		return uuid.Nil, errors.New("no token provided")
	}

	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

	token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("JWT_SECRET")), nil
	})

	if err != nil || !token.Valid {
		return uuid.Nil, errors.New("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return uuid.Nil, errors.New("invalid token claims")
	}

	userIdStr, ok := claims["user_id"].(string)
	if !ok {
		return uuid.Nil, errors.New("user id not found in token")
	}

	return uuid.Parse(userIdStr)
}

// IsAdmin checks if the user is an admin
func IsAdmin(ctx context.Context, userID uuid.UUID) (bool, error) {
	// Check if user is an admin, check user database table 'is_admin' column if true or false
	db, err := database.NewDatabaseConnection(ctx)
	if err != nil {
		return false, err
	}
	defer db.Close()

	var isAdmin bool
	err = db.QueryRow(ctx, `
		SELECT is_admin FROM users WHERE id = $1
	`, userID).Scan(&isAdmin)

	if err != nil {
		return false, err
	}

	return isAdmin, nil
}

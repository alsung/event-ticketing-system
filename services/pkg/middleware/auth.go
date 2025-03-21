package middleware

import (
	"errors"
	"net/http"
	"os"
	"strings"

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

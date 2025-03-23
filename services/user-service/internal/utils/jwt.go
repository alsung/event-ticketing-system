package utils

import (
	"os"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
)

func GenerateJWT(userID uuid.UUID, email string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"email":   email,
		"user_id": userID.String(),
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})

	return token.SignedString([]byte(os.Getenv("JWT_SECRET")))
}

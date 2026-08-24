package utils

import (
	"os"

	"github.com/alsung/event-ticketing-system/services/pkg/auth"
	"github.com/google/uuid"
)

// GenerateJWT issues a login token.
//
// Signing lives in pkg/auth so that the code issuing tokens and the code
// verifying them are the same implementation against the same library version.
// This previously signed with golang-jwt v3 while the gateway verified with v5.
func GenerateJWT(userID uuid.UUID, email string) (string, error) {
	return auth.NewSigner(os.Getenv("JWT_SECRET"), auth.DefaultTTL).Sign(userID, email)
}

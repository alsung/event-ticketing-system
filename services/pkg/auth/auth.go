// Package auth issues and verifies the JWTs that carry caller identity between
// the gateway and the services behind it.
//
// It deliberately depends on nothing but the JWT library and a UUID type. The
// API gateway imports this package, and pulling a database driver or an HTTP
// framework in here would put those in the gateway's binary too.
package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// DefaultTTL is how long an issued token stays valid.
const DefaultTTL = 24 * time.Hour

var (
	ErrNoToken      = errors.New("auth: no bearer token provided")
	ErrInvalidToken = errors.New("auth: invalid token")
)

// Claims is the identity this system carries in a token.
type Claims struct {
	UserID uuid.UUID
	Email  string
}

// Signer issues tokens. Only user-service should hold one.
type Signer struct {
	secret []byte
	ttl    time.Duration
}

func NewSigner(secret string, ttl time.Duration) *Signer {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Signer{secret: []byte(secret), ttl: ttl}
}

func (s *Signer) Sign(userID uuid.UUID, email string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID.String(),
		"email":   email,
		"iat":     now.Unix(),
		"exp":     now.Add(s.ttl).Unix(),
	})
	return token.SignedString(s.secret)
}

// Verifier validates tokens. Every service holds one.
type Verifier struct {
	secret []byte
}

func NewVerifier(secret string) *Verifier {
	return &Verifier{secret: []byte(secret)}
}

// Verify parses and validates a raw token string.
//
// The signing method is pinned to HMAC. A JWT header declares its own algorithm,
// and a parser that trusts that declaration accepts two classic forgeries: a
// token with alg "none" and no signature at all, and an RS256 token whose public
// key -- public by definition -- is replayed as the HMAC secret. Checking the
// concrete method type before returning the key defeats both.
func (v *Verifier) Verify(tokenStr string) (*Claims, error) {
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return v.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))

	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	rawID, ok := mapClaims["user_id"].(string)
	if !ok {
		return nil, fmt.Errorf("%w: no user_id claim", ErrInvalidToken)
	}
	userID, err := uuid.Parse(rawID)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed user_id claim", ErrInvalidToken)
	}

	email, _ := mapClaims["email"].(string)
	return &Claims{UserID: userID, Email: email}, nil
}

// FromRequest pulls the bearer token off a request and verifies it.
func (v *Verifier) FromRequest(r *http.Request) (*Claims, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return nil, ErrNoToken
	}
	if !strings.HasPrefix(header, "Bearer ") {
		return nil, ErrNoToken
	}
	return v.Verify(strings.TrimPrefix(header, "Bearer "))
}

// DefaultVerifier returns a process-wide verifier built from JWT_SECRET.
//
// Handlers are package-level functions today, so there is nowhere to inject a
// verifier; Phase 1 pass B converts them to structs and this can go away.
func DefaultVerifier() *Verifier {
	defaultOnce.Do(func() {
		defaultVerifier = NewVerifier(os.Getenv("JWT_SECRET"))
	})
	return defaultVerifier
}

var (
	defaultOnce     sync.Once
	defaultVerifier *Verifier
)

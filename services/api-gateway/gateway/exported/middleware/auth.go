package middleware

import (
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/auth"
)

// JWTMiddleware verifies the bearer token on protected routes. GatewayHandler
// applies it per-route, so public routes never reach it.
//
// Verification lives in pkg/auth rather than here. This file used to carry its
// own jwt.Parse call against a different major version of the JWT library than
// the services behind it used, which meant two code paths for one security
// decision and only this one pinned the signing algorithm.
//
// CORS headers are deliberately not set here: CORSMiddleware wraps this one and
// owns them for every response, including the 401s produced below.
func JWTMiddleware(next http.Handler) http.Handler {
	verifier := auth.DefaultVerifier()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.FromRequest(r); err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

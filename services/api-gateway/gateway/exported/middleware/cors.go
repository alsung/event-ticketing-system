package middleware

import "net/http"

// CORSMiddleware handles CORS preflight requests and sets response headers
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers for all requests
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		// Idempotency-Key is not a CORS-safelisted header, so omitting it here
		// makes the browser's preflight reject every purchase and cancellation
		// from the frontend -- silently, since the request never leaves. curl
		// does not preflight, which is why this survived every API-level test.
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key")

		// Handle preflight (OPTIONS) requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

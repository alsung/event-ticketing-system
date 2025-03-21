package middleware

import (
	"log"
	"net/http"
	"time"
)

// Logging middleware provides consistent logging across services
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("[REQ] %s %s %s", r.RemoteAddr, r.Method, r.URL.Path)

		next.ServeHTTP(w, r)

		log.Printf("[RES] %s %s - %v", r.Method, r.URL.Path, time.Since(start))
	})
}

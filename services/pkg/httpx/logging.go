// Package httpx holds HTTP plumbing shared by every service.
//
// It depends on the standard library only. The API gateway imports this package,
// so anything added here lands in the gateway's binary -- keep database drivers,
// message-queue clients and payment SDKs out.
package httpx

import (
	"log"
	"net/http"
	"time"
)

// statusRecorder captures the response code so it can be logged. http.ResponseWriter
// does not expose what was written, so the only way to observe it is to wrap it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Logging records the method, path, status and duration of each request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		log.Printf("%s %s %d %s %s",
			r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Microsecond), r.RemoteAddr)
	})
}

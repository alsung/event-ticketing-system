package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// Check is a named dependency probe.
type Check func(context.Context) error

// Live answers whether the process is running at all. It touches no
// dependencies, so it stays 200 while the database is down -- restarting a
// process because Postgres is unreachable would not help and would take out
// every replica at once.
func Live() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// Ready answers whether the process can serve traffic, which means its
// dependencies are reachable. A load balancer should stop routing here while
// this fails, without killing the process.
func Ready(checks map[string]Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		results := make(map[string]string, len(checks))
		status := http.StatusOK

		for name, check := range checks {
			if err := check(ctx); err != nil {
				results[name] = err.Error()
				status = http.StatusServiceUnavailable
				continue
			}
			results[name] = "ok"
		}

		body := map[string]any{"checks": results}
		if status == http.StatusOK {
			body["status"] = "ok"
		} else {
			body["status"] = "unavailable"
		}
		writeJSON(w, status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// SelfCheck performs an HTTP readiness probe against the local process and exits
// non-zero on failure.
//
// This exists so a Docker HEALTHCHECK can run inside a distroless image, which
// ships no shell and no curl. The service binary probes itself:
//
//	HEALTHCHECK CMD ["/user-service", "-healthcheck"]
func SelfCheck(port string) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health/ready", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

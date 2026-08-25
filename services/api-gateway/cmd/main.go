package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported"
	exportedMiddleware "github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported/middleware"
	"github.com/alsung/event-ticketing-system/services/pkg/httpx"
	"github.com/joho/godotenv"
)

const port = "8000"

func main() {
	// Run as a health probe instead of a server when asked. Docker HEALTHCHECK
	// uses this because the distroless runtime image has no shell or curl.
	healthcheck := flag.Bool("healthcheck", false, "probe local readiness and exit")
	flag.Parse()
	if *healthcheck {
		httpx.SelfCheck(port)
	}

	// .env is a local-development convenience. Under Docker Compose the
	// environment is supplied by the compose file, so a missing file is normal.
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded, using environment variables")
	}

	gatewayHandler := exported.NewGatewayHandler()

	// Logging -> CORS -> gateway (which applies JWT per-route).
	//
	// CORS sits outside auth so that preflight OPTIONS short-circuits before any
	// token check, and so 401 responses still carry the headers the browser needs
	// to surface the real status instead of an opaque network error.
	proxied := httpx.Logging(
		exportedMiddleware.CORSMiddleware(gatewayHandler),
	)

	// The gateway owns no database, so readiness means the services it fronts are
	// answering. Registering these on the mux ahead of the catch-all keeps them
	// from being proxied downstream as if they were application routes.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", httpx.Live())
	mux.HandleFunc("/health/ready", httpx.Ready(map[string]httpx.Check{
		"user-service":   upstreamCheck("USER_SERVICE_URL"),
		"event-service":  upstreamCheck("EVENT_SERVICE_URL"),
		"ticket-service": upstreamCheck("TICKET_SERVICE_URL"),
	}))
	mux.Handle("/", proxied)

	log.Println("api-gateway listening on :" + port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// upstreamCheck probes a downstream service's liveness endpoint.
//
// It deliberately calls /health rather than /health/ready: readiness should not
// cascade. If Postgres is down, every service reports unready, and a gateway that
// chained their readiness would report unready too -- taking the whole edge out
// of rotation over one shared dependency, and hiding which layer actually broke.
func upstreamCheck(envVar string) httpx.Check {
	base := os.Getenv(envVar)
	client := &http.Client{Timeout: 2 * time.Second}

	return func(ctx context.Context) error {
		if base == "" {
			return fmt.Errorf("%s is not set", envVar)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	}
}

package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/event-service/internal/handlers"
	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/pkg/httpx"
	"github.com/joho/godotenv"
)

const port = "8082"

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

	// Open the pool once at startup so a failure is visible immediately rather
	// than on the first request.
	if _, err := database.Pool(context.Background()); err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/health", httpx.Live())
	mux.HandleFunc("/health/ready", httpx.Ready(map[string]httpx.Check{
		"postgres": database.Ping,
	}))
	// Every pattern names its method. Without one, "/events/create" also matches
	// GET /events/create, which "GET /events/{id}" matches too -- neither is a
	// strict subset of the other, so ServeMux treats it as a conflict and panics
	// at registration rather than guessing.
	mux.HandleFunc("GET /events", handlers.GetEvents)
	mux.HandleFunc("POST /events/create", handlers.CreateEvent)
	mux.HandleFunc("GET /events/{id}", handlers.GetEvent)
	mux.HandleFunc("PUT /events/{id}", handlers.UpdateEvent)

	// Organiser routes sit under their own prefix rather than as /events/mine,
	// which removes the same ambiguity by construction and gives the gateway a
	// single prefix to protect.
	mux.HandleFunc("GET /organizer/events", handlers.MyEvents)

	handlerWithMiddleware := httpx.Logging(mux)

	log.Println("event-service listening on :" + port)
	if err := http.ListenAndServe(":"+port, handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

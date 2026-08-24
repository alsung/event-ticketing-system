package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/event-service/internal/handlers"
	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/joho/godotenv"
)

func main() {
	// .env is a local-development convenience. Under Docker Compose the
	// environment is supplied by the compose file, so a missing file is normal.
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded, using environment variables")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/events", handlers.GetEvents)          // GET events
	mux.HandleFunc("/events/create", handlers.CreateEvent) // POST create events

	handlerWithMiddleware := middleware.Logging(mux)

	log.Println("Event service running on :8082")
	if err := http.ListenAndServe(":8082", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

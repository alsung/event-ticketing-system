package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/event-service/internal/handlers"
	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.GetEvents)         // GET events
	mux.HandleFunc("/create", handlers.CreateEvent) // POST create events

	handlerWithMiddleware := middleware.Logging(mux)

	log.Println("Event service running on :8082")
	if err := http.ListenAndServe(":8082", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

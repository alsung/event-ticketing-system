package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/handlers"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/purchase", handlers.PurchaseTicket)
	mux.HandleFunc("/list", handlers.ListAvailableTickets)

	handlerWithMiddleware := middleware.Logging(mux)

	log.Println("Ticket service running on :8083")
	if err := http.ListenAndServe(":8083", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

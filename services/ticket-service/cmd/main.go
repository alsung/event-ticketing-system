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

	mux.HandleFunc("/tickets/purchase", handlers.PurchaseTicket)
	mux.HandleFunc("/tickets/create", handlers.CreateTickets)
	mux.HandleFunc("/tickets/available", handlers.ListAvailableTickets)
	mux.HandleFunc("/tickets/mine", handlers.GetUserTickets)
	// mux.HandleFunc("/tickets/cancel", handlers.CancelTicket)
	// mux.HandleFunc("/tickets/purchased", handlers.ListPurchasedTickets)

	handlerWithMiddleware := middleware.Logging(mux)

	log.Println("Ticket service running on :8083")
	if err := http.ListenAndServe(":8083", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

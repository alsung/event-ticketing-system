package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/handlers"
	"github.com/joho/godotenv"
)

func main() {
	// .env is a local-development convenience. Under Docker Compose the
	// environment is supplied by the compose file, so a missing file is normal.
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded, using environment variables")
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/tickets/purchase", handlers.PurchaseTicket)
	mux.HandleFunc("/tickets/create", handlers.CreateTickets)
	mux.HandleFunc("/tickets/available", handlers.ListAvailableTickets)
	mux.HandleFunc("/tickets/mine", handlers.GetUserTickets)
	mux.HandleFunc("/tickets/cancel", handlers.CancelTicket)
	mux.HandleFunc("/tickets/receipt", handlers.GetTicketReceipt)
	// mux.HandleFunc("/tickets/purchased", handlers.ListPurchasedTickets)

	handlerWithMiddleware := middleware.Logging(mux)

	log.Println("Ticket service running on :8083")
	if err := http.ListenAndServe(":8083", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

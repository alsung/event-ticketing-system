package main

import (
	"context"
	"flag"
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/pkg/httpx"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/handlers"
	"github.com/joho/godotenv"
)

const port = "8083"

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

	mux.HandleFunc("/tickets/purchase", handlers.PurchaseTicket)
	mux.HandleFunc("/tickets/create", handlers.CreateTickets)
	mux.HandleFunc("/tickets/available", handlers.ListAvailableTickets)
	mux.HandleFunc("/tickets/mine", handlers.GetUserTickets)
	mux.HandleFunc("/tickets/cancel", handlers.CancelTicket)
	mux.HandleFunc("/tickets/receipt", handlers.GetTicketReceipt)
	// mux.HandleFunc("/tickets/purchased", handlers.ListPurchasedTickets)

	handlerWithMiddleware := httpx.Logging(mux)

	log.Println("ticket-service listening on :" + port)
	if err := http.ListenAndServe(":"+port, handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported"
	exportedMiddleware "github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported/middleware"
	"github.com/alsung/event-ticketing-system/services/pkg/httpx"
	"github.com/joho/godotenv"
)

func main() {
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
	handler := httpx.Logging(
		exportedMiddleware.CORSMiddleware(gatewayHandler),
	)

	http.Handle("/", handler)

	log.Println("API Gateway running on :8000")
	if err := http.ListenAndServe(":8000", nil); err != nil {
		log.Fatal(err)
	}
}

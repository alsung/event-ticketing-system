package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported"
	exportedMiddleware "github.com/alsung/event-ticketing-system/services/api-gateway/gateway/exported/middleware"
	sharedMiddleware "github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	gatewayHandler := exported.NewGatewayHandler()

	// Apply middleware: CORS -> JWT -> Logging
	handler := exportedMiddleware.CORSMiddleware(gatewayHandler)
	handler = exportedMiddleware.JWTMiddleware(handler)
	handler = sharedMiddleware.Logging(handler)

	http.Handle("/", handler)

	log.Println("API Gateway running on :8000")
	if err := http.ListenAndServe(":8000", nil); err != nil {
		log.Fatal(err)
	}
}

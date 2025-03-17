package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/api-gateway/gateway/exported"
	"github.com/alsung/event-ticketing-system/api-gateway/gateway/exported/middleware"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	// Initialize Gateway through exported function (public API)
	gatewayHandler := exported.NewGatewayHandler()

	// Wrap the handler with JWT middleware
	handlerWithMiddleware := middleware.JWTMiddleware(gatewayHandler)

	http.Handle("/", handlerWithMiddleware)

	log.Println("API Gateway running on :8000")
	if err := http.ListenAndServe(":8000", nil); err != nil {
		log.Fatal(err)
	}
}

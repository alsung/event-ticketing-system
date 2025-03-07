package main

import (
	"log"

	"github.com/alsung/event-ticketing-system/api-gateway/internal/handlers"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	router := gin.Default()

	// CORS middleware
	router.Use(cors.Default())

	// Setup API Gateway routes
	handlers.RegisterGatewayRoutes(router)

	router.Run(":8000")
}

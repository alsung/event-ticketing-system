package main

import (
	"log"

	"github.com/alsung/event-ticketing-system/event-service/internal/handlers"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	router := gin.Default()

	router.POST("/events", handlers.CreateEvent)
	router.GET("/events", handlers.GetEvents)

	router.Run(":8082") // Event Service running on port 8082
}

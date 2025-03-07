package main

import (
	"log"

	"github.com/alsung/event-ticketing-system/user-service/internal/handlers"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	router := gin.Default()

	router.POST("/register", handlers.RegisterUser)
	router.POST("/login", handlers.LoginUser)

	router.Run(":8081")
}

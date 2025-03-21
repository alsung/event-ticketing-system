package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/middleware"
	"github.com/alsung/event-ticketing-system/services/user-service/internal/handlers"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Failed to load .env file")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/register", handlers.RegisterUser)
	mux.HandleFunc("/login", handlers.LoginUser)

	handlerWithMiddleware := middleware.Logging(mux)

	log.Println("User Service running on :8081")
	if err := http.ListenAndServe(":8081", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

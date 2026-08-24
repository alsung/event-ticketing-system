package main

import (
	"log"
	"net/http"

	"github.com/alsung/event-ticketing-system/services/pkg/httpx"
	"github.com/alsung/event-ticketing-system/services/user-service/internal/handlers"
	"github.com/joho/godotenv"
)

func main() {
	// .env is a local-development convenience. Under Docker Compose the
	// environment is supplied by the compose file, so a missing file is normal.
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded, using environment variables")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/users/register", handlers.RegisterUser)
	mux.HandleFunc("/users/login", handlers.LoginUser)

	handlerWithMiddleware := httpx.Logging(mux)

	log.Println("User Service running on :8081")
	if err := http.ListenAndServe(":8081", handlerWithMiddleware); err != nil {
		log.Fatal(err)
	}
}

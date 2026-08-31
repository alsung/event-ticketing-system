// Command worker consumes ticket events and records notifications.
//
// A separate binary from the HTTP server on purpose: it joins its own Kafka
// consumer group, so it can be scaled, restarted and lag-monitored
// independently of request traffic. It lives in the ticket-service module
// rather than a fifth Go module, since it shares the event definitions.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/alsung/event-ticketing-system/services/pkg/database"
	"github.com/alsung/event-ticketing-system/services/ticket-service/internal/outbox"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded, using environment variables")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if _, err := database.Pool(ctx); err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	// Subscribing to a topic that does not exist yet leaves the group with no
	// partitions assigned, and it does not recover on its own.
	if err := outbox.WaitForTopics(ctx); err != nil {
		log.Fatalf("kafka: %v", err)
	}

	outbox.NewNotificationConsumer().Run(ctx)
}

package main

import (
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

// mq is the package-level RabbitMQ client shared by the webhook handler and
// the consumer. It is initialised in main before the HTTP server starts.
var mq *RabbitMQ

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(".env"); err != nil {
		log.Println("Warning: .env file not found, checking system environment variables")
	} else {
		log.Println("✓ Successfully loaded .env file")
	}

	// Verify environment variables are loaded
	appID := getAppIDFromEnv()
	if appID != "" {
		log.Println("✓ GITHUB_APP_ID is set:", appID)
	} else {
		log.Println("⚠ Warning: GITHUB_APP_ID is not set")
	}

	// Connect to RabbitMQ and start the async consumer.
	rabbitmqURL := os.Getenv("RABBITMQ_URL")
	if rabbitmqURL == "" {
		rabbitmqURL = "amqp://guest:guest@localhost:5672/"
	}
	var err error
	mq, err = NewRabbitMQ(rabbitmqURL)
	if err != nil {
		log.Printf("Warning: could not connect to RabbitMQ (%s): %v — webhook events will be dropped\n", rabbitmqURL, err)
	} else {
		log.Println("Connected to RabbitMQ:", rabbitmqURL)
		go StartConsumer(mq)
		go StartEventBusConsumer(mq)
		defer mq.Close()
	}

	// Register HTTP routes
	http.HandleFunc("/", handler)
	http.HandleFunc("/webhook", WebhookHandler)
	http.HandleFunc("/auth-test", AuthTestHandler)
	http.HandleFunc("/repo-files", GetRepositoryFilesHandler)
	http.HandleFunc("/pr-files", GetPRFilesHandler)

	// Log startup information
	log.Println("listening on Port 3000")
	log.Println("Available endpoints:")
	log.Println("  GET/POST /          - Basic handler")
	log.Println("  POST     /webhook    - GitHub webhook handler")
	log.Println("  GET      /auth-test  - GitHub App authentication test")
	log.Println("  GET      /repo-files - Get repository file list (requires ?owner=X&repo=Y)")
	log.Println("  GET      /pr-files   - Get PR changed files (requires ?owner=X&repo=Y&pr=N)")

	// Start server
	log.Fatal(http.ListenAndServe(":3000", nil))
}

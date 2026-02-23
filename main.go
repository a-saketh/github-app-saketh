package main

import (
	"log"
	"net/http"

	"github.com/joho/godotenv"
)

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

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// verifyWebhookSignature validates the GitHub webhook signature
func verifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// GitHub uses X-Hub-Signature-256: sha256=<signature>
	// Remove "sha256=" prefix if present
	if strings.HasPrefix(signature, "sha256=") {
		signature = signature[7:]
	}

	// Create HMAC SHA256 hash
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))

	// Compare signatures (constant time comparison for security)
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

// WebhookHandler processes incoming GitHub webhooks
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("webhook received!!")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusInternalServerError)
		return
	}

	// Verify webhook signature
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Println("Error: WEBHOOK_SECRET environment variable not set")
		http.Error(w, "webhook secret not configured", http.StatusInternalServerError)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		log.Println("Error: X-Hub-Signature-256 header missing")
		http.Error(w, "signature missing", http.StatusBadRequest)
		return
	}

	if !verifyWebhookSignature(body, signature, webhookSecret) {
		log.Println("Error: Invalid webhook signature - verification failed")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	log.Println("Webhook signature verified successfully!")

	// Print event type
	eventType := r.Header.Get("X-GitHub-Event")
	log.Println("Event type:", eventType)

	// Parse webhook payload
	var payload WebhookPayload
	err = json.Unmarshal(body, &payload)
	if err != nil {
		log.Println("Error: Failed to parse webhook payload:", err)
		http.Error(w, "invalid payload format", http.StatusBadRequest)
		return
	}

	// Extract PR details
	prTitle := payload.PullRequest.Title
	prNumber := payload.PullRequest.Number
	prCreator := payload.PullRequest.User.Login
	repoOwner := payload.Repository.Owner.Login
	repoName := payload.Repository.Name

	// Log extracted information
	log.Println("=== Extracted PR Details ===")
	log.Println("PR Title:", prTitle)
	log.Println("PR Number:", prNumber)
	log.Println("PR Creator:", prCreator)
	log.Println("Repository Owner:", repoOwner)
	log.Println("Repository Name:", repoName)

	// Fetch changed files for PR events
	if eventType == "pull_request" && prNumber != 0 {
		log.Println("Fetching changed files for PR...")

		appID := getAppIDFromEnv()
		privateKey := getPrivateKeyFromEnv()

		if appID != "" && privateKey != "" {
			jwtToken, err := generateJWT(appID, privateKey)
			if err == nil {
				installationToken, err := getInstallationToken(jwtToken, repoOwner, repoName)
				if err == nil {
					files, err := getPRChangedFiles(installationToken, repoOwner, repoName, prNumber)
					if err == nil {
						logPRChangedFiles(files)
					} else {
						log.Println("Warning: Could not fetch PR files:", err)
					}
				} else {
					log.Println("Warning: Could not get installation token:", err)
				}
			} else {
				log.Println("Warning: Could not generate JWT:", err)
			}
		} else {
			log.Println("Warning: GitHub App credentials not configured")
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("received"))
}

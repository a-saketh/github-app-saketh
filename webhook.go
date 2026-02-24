package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// verifyWebhookSignature validates the HMAC-SHA256 signature attached to a
// webhook payload. Works for both GitHub (X-Hub-Signature-256) and Bitbucket
// (X-Hub-Signature) because both use the same algorithm.
func verifyWebhookSignature(payload []byte, signature string, secret string) bool {
	// Strip the "sha256=" prefix that GitHub and Bitbucket both include.
	if strings.HasPrefix(signature, "sha256=") {
		signature = signature[7:]
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expected := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// WebhookHandler is the single HTTP endpoint that receives webhooks from any
// supported SCM platform (GitHub, Bitbucket).
//
// Processing flow (mirrors the sequence diagram):
//  1. Read and verify the HMAC signature  → reject invalid payloads early.
//  2. Detect which SCM platform sent the event.
//  3. Return 200 OK immediately  (non-blocking acknowledgement to the SCM).
//  4. Publish the raw event to RabbitMQ (raw_webhook_events queue).
//     The SCM Adapter consumer picks it up asynchronously, normalizes it,
//     and forwards it to the Unified Event Bus (normalized_pr_events queue).
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	log.Println("=== Webhook received ===")

	// --- Step 1: Read body ---
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "cannot read body", http.StatusInternalServerError)
		return
	}

	// --- Step 2: Verify signature ---
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	if webhookSecret == "" {
		log.Println("Error: WEBHOOK_SECRET environment variable not set")
		http.Error(w, "webhook secret not configured", http.StatusInternalServerError)
		return
	}

	// GitHub uses X-Hub-Signature-256; Bitbucket uses X-Hub-Signature.
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		signature = r.Header.Get("X-Hub-Signature")
	}
	if signature == "" {
		log.Println("Error: webhook signature header missing")
		http.Error(w, "signature missing", http.StatusBadRequest)
		return
	}
	if !verifyWebhookSignature(body, signature, webhookSecret) {
		log.Println("Error: webhook signature verification failed")
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	log.Println("Signature verified successfully")

	// --- Step 3: Detect platform ---
	platform := DetectPlatform(r.Header)
	log.Printf("Detected SCM platform: %s\n", platform)

	// Resolve the raw event-type string from the appropriate header.
	eventType := r.Header.Get("X-GitHub-Event") // GitHub
	if platform == PlatformBitbucket {
		eventType = r.Header.Get("X-Event-Key") // Bitbucket
	}
	log.Printf("Event type: %s\n", eventType)

	// --- Step 4: Acknowledge immediately ---
	// The SCM expects a fast 200 OK. All further processing happens after the
	// response is sent, keeping the webhook round-trip non-blocking.
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("received"))

	// --- Step 5: Skip non-PR events ---
	isPREvent := eventType == "pull_request" || strings.HasPrefix(eventType, "pullrequest:")
	if !isPREvent {
		log.Printf("Skipping non-PR event: %s\n", eventType)
		return
	}

	// --- Step 6: Publish raw event to the message queue ---
	if mq == nil {
		log.Println("Warning: RabbitMQ not initialised, raw event dropped")
		return
	}

	msg := RawWebhookMessage{
		Platform:  platform,
		EventType: eventType,
		Payload:   body,
	}
	if err := mq.PublishRawEvent(msg); err != nil {
		log.Printf("Warning: could not publish raw event to queue: %v\n", err)
	}
}

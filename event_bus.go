package main

// Unified Event Bus — publishes normalized events to the Platform BE.
//
// Receives normalized events from the SCM Adapter consumer and delivers them
// to the Platform BE via HTTP POST. If PLATFORM_BE_URL is not configured
// (e.g. local development), the event is logged instead.
//
// This is the final step in the pipeline:
//
//	SCM Adapter → Normalize → Unified Event Bus → Platform BE

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// DeliverEvent sends a normalized event to the Platform BE via HTTP POST.
//
// If url is empty (PLATFORM_BE_URL not configured), the event is logged only —
// useful for local development where no Platform BE is running.
//
// Mirrors the Python publish() function:
//   - No URL  → log the event and return (dev mode).
//   - HTTP 4xx/5xx → log the status code and response body as an error.
//   - Network error → log and return the error.
func DeliverEvent(event *NormalizedEvent, url string) error {
	if url == "" {
		// Dev mode: no Platform BE configured — log the full normalized event.
		log.Printf("[EventBus] PLATFORM_BE_URL not set — normalized event (PR #%d, platform=%s, action=%s)\n",
			event.PR.Number, event.Platform, event.Action)
		return nil
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("event_bus: failed to marshal event: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		// Mirrors Python's httpx.RequestError branch.
		return fmt.Errorf("event_bus: failed to reach Platform BE at %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Drain the body so the connection can be reused.
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		// Mirrors Python's httpx.HTTPStatusError branch.
		return fmt.Errorf("event_bus: Platform BE returned error %d for %s: %s",
			resp.StatusCode, url, string(respBody))
	}

	log.Printf("[EventBus] Delivered normalized event to Platform BE — url=%s status=%d\n",
		url, resp.StatusCode)
	return nil
}

// StartEventBusConsumer begins consuming normalized events from the
// normalized_pr_events queue (the "Unified Event Bus") and delivers each one
// to the Platform BE.
//
// Reads PLATFORM_BE_URL from the environment at startup. If the variable is
// not set, events are logged only (dev mode) — matching the Python behaviour.
//
// This function blocks until the broker closes the channel; call it in a
// goroutine from main.
func StartEventBusConsumer(mq *RabbitMQ) {
	platformBEURL := os.Getenv("PLATFORM_BE_URL")
	if platformBEURL == "" {
		log.Println("[EventBus] PLATFORM_BE_URL not set — events will be logged only (dev mode)")
	} else {
		log.Printf("[EventBus] Delivering normalized events to Platform BE at %s\n", platformBEURL)
	}

	if err := mq.ConsumeNormalizedEvents(func(event *NormalizedEvent) {
		if err := DeliverEvent(event, platformBEURL); err != nil {
			log.Printf("[EventBus] Warning: could not deliver event (PR #%d): %v\n",
				event.PR.Number, err)
		}
	}); err != nil {
		log.Fatalf("[EventBus] Fatal error, consumer stopped: %v\n", err)
	}
}

package main

import (
	"log"
)

// StartConsumer begins consuming raw webhook events from the RabbitMQ queue
// and runs the full SCM Adapter pipeline for each one:
//
//  1. Identify platform (already encoded in the message).
//  2. Build the right SCMAdapter.
//  3. Call NormalizeEvent — fetches PR metadata + changed files from the SCM API.
//  4. Publish the resulting NormalizedEvent to the normalized events queue
//     (the "Unified Event Bus" in the sequence diagram).
//
// This function blocks until the broker closes the channel; call it in a
// goroutine from main.
func StartConsumer(mq *RabbitMQ) {
	if err := mq.ConsumeRawEvents(processRawEvent(mq)); err != nil {
		log.Fatalf("[Consumer] Fatal error, consumer stopped: %v\n", err)
	}
}

// processRawEvent returns a closure that handles a single RawWebhookMessage
// through the SCM Adapter pipeline.
func processRawEvent(mq *RabbitMQ) func(RawWebhookMessage) {
	return func(msg RawWebhookMessage) {
		log.Printf("[Consumer] Received event — platform=%s type=%s\n", msg.Platform, msg.EventType)

		// Build the adapter for the detected platform.
		adapter, err := NewSCMAdapter(msg.Platform)
		if err != nil {
			log.Printf("[Consumer] Warning: could not create adapter for %q: %v\n", msg.Platform, err)
			return
		}

		// NormalizeEvent parses the payload, fetches PR details and files from
		// the SCM API, and returns a platform-agnostic NormalizedEvent.
		event, err := adapter.NormalizeEvent(msg.EventType, msg.Payload)
		if err != nil {
			log.Printf("[Consumer] Warning: could not normalize event: %v\n", err)
			return
		}

		logNormalizedEvent(event)

		// Publish to the Unified Event Bus (normalized_pr_events queue).
		if err := mq.PublishNormalizedEvent(event); err != nil {
			log.Printf("[Consumer] Warning: could not publish normalized event: %v\n", err)
		}
	}
}

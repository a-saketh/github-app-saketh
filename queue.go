package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	rawEventsQueue        = "raw_webhook_events"
	normalizedEventsQueue = "normalized_pr_events"
)

// RawWebhookMessage is the message published to the raw events queue by the
// Webhook Gateway. It carries everything the SCM Adapter needs to process the
// event without access to the original HTTP request.
type RawWebhookMessage struct {
	Platform  SCMPlatform `json:"platform"`
	EventType string      `json:"event_type"`
	Payload   []byte      `json:"payload"`
}

// RabbitMQ wraps an AMQP connection and a dedicated publish channel.
// Each consumer (ConsumeRawEvents, ConsumeNormalizedEvents) opens its own
// channel so that concurrent goroutines never share a single channel —
// amqp091-go channels are not goroutine-safe.
type RabbitMQ struct {
	conn      *amqp.Connection
	publishMu sync.Mutex   // guards pubCh across concurrent HTTP handler goroutines
	pubCh     *amqp.Channel // used exclusively for publishing
}

// NewRabbitMQ dials the broker at url, opens a dedicated publish channel, and
// declares the two durable queues the application uses.
func NewRabbitMQ(url string) (*RabbitMQ, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: failed to connect to %s: %w", url, err)
	}

	pubCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: failed to open publish channel: %w", err)
	}

	mq := &RabbitMQ{conn: conn, pubCh: pubCh}
	if err := mq.declareQueues(pubCh); err != nil {
		mq.Close()
		return nil, err
	}

	return mq, nil
}

// declareQueues ensures both application queues exist on the broker.
// Durable queues survive a broker restart; messages marked Persistent also
// survive if they were written to disk before the restart.
func (mq *RabbitMQ) declareQueues(ch *amqp.Channel) error {
	for _, name := range []string{rawEventsQueue, normalizedEventsQueue} {
		if _, err := ch.QueueDeclare(
			name,  // queue name
			true,  // durable
			false, // auto-delete when unused
			false, // exclusive
			false, // no-wait
			nil,   // additional arguments
		); err != nil {
			return fmt.Errorf("rabbitmq: failed to declare queue %q: %w", name, err)
		}
		log.Printf("[RabbitMQ] Queue declared: %q\n", name)
	}
	return nil
}

// PublishRawEvent serialises msg as JSON and sends it to the raw events queue.
// Called by the Webhook Gateway immediately after signature verification.
// The mutex ensures safe concurrent calls from multiple HTTP handler goroutines.
func (mq *RabbitMQ) PublishRawEvent(msg RawWebhookMessage) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("rabbitmq: failed to marshal raw event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mq.publishMu.Lock()
	defer mq.publishMu.Unlock()

	if err := mq.pubCh.PublishWithContext(ctx,
		"",             // default exchange
		rawEventsQueue, // routing key = queue name
		false,          // mandatory
		false,          // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent, // survive broker restart
			Body:         body,
		},
	); err != nil {
		return fmt.Errorf("rabbitmq: failed to publish raw event: %w", err)
	}

	log.Printf("[RabbitMQ] Published raw event (platform=%s, type=%s) to %q\n",
		msg.Platform, msg.EventType, rawEventsQueue)
	return nil
}

// PublishNormalizedEvent serialises event as JSON and sends it to the
// normalized events queue (the "Unified Event Bus" in the sequence diagram).
// Called by the SCM Adapter consumer after normalization.
func (mq *RabbitMQ) PublishNormalizedEvent(event *NormalizedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("rabbitmq: failed to marshal normalized event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	mq.publishMu.Lock()
	defer mq.publishMu.Unlock()

	if err := mq.pubCh.PublishWithContext(ctx,
		"",                    // default exchange
		normalizedEventsQueue, // routing key = queue name
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         body,
		},
	); err != nil {
		return fmt.Errorf("rabbitmq: failed to publish normalized event: %w", err)
	}

	log.Printf("[RabbitMQ] Published normalized event (PR #%d) to %q\n",
		event.PR.Number, normalizedEventsQueue)
	return nil
}

// ConsumeRawEvents opens a dedicated channel, registers a consumer on the raw
// events queue, and calls handler for every delivery. Each consumer goroutine
// gets its own channel so it never races with the publish channel or the other
// consumer goroutine.
//
// This method blocks until the channel is closed; run it in a goroutine.
func (mq *RabbitMQ) ConsumeRawEvents(handler func(RawWebhookMessage)) error {
	ch, err := mq.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq: failed to open consumer channel for %q: %w", rawEventsQueue, err)
	}
	defer ch.Close()

	deliveries, err := ch.Consume(
		rawEventsQueue, // queue
		"",             // consumer tag (auto-generated)
		false,          // auto-ack disabled — we ack manually
		false,          // exclusive
		false,          // no-local
		false,          // no-wait
		nil,            // arguments
	)
	if err != nil {
		return fmt.Errorf("rabbitmq: failed to register consumer on %q: %w", rawEventsQueue, err)
	}

	log.Printf("[RabbitMQ] Consumer started, listening on queue %q\n", rawEventsQueue)

	for d := range deliveries {
		var msg RawWebhookMessage
		if err := json.Unmarshal(d.Body, &msg); err != nil {
			log.Printf("[RabbitMQ] Warning: could not decode delivery, discarding: %v\n", err)
			d.Nack(false, false) // discard; requeue=false avoids poison-message loop
			continue
		}
		handler(msg)
		d.Ack(false)
	}

	return nil
}

// ConsumeNormalizedEvents opens a dedicated channel, registers a consumer on
// the normalized events queue, and calls handler for every delivery. Mirrors
// ConsumeRawEvents but operates on the normalizedEventsQueue.
//
// This method blocks until the channel is closed; run it in a goroutine.
func (mq *RabbitMQ) ConsumeNormalizedEvents(handler func(*NormalizedEvent)) error {
	ch, err := mq.conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq: failed to open consumer channel for %q: %w", normalizedEventsQueue, err)
	}
	defer ch.Close()

	deliveries, err := ch.Consume(
		normalizedEventsQueue, // queue
		"",                    // consumer tag (auto-generated)
		false,                 // auto-ack disabled — we ack manually
		false,                 // exclusive
		false,                 // no-local
		false,                 // no-wait
		nil,                   // arguments
	)
	if err != nil {
		return fmt.Errorf("rabbitmq: failed to register consumer on %q: %w", normalizedEventsQueue, err)
	}

	log.Printf("[RabbitMQ] Consumer started, listening on queue %q\n", normalizedEventsQueue)

	for d := range deliveries {
		var event NormalizedEvent
		if err := json.Unmarshal(d.Body, &event); err != nil {
			log.Printf("[RabbitMQ] Warning: could not decode normalized event, discarding: %v\n", err)
			d.Nack(false, false) // discard; requeue=false avoids poison-message loop
			continue
		}
		handler(&event)
		d.Ack(false)
	}

	return nil
}

// Close releases the publish channel and the underlying connection.
// Consumer channels are self-managed (opened and closed inside each Consume
// method), so they do not need to be closed here.
func (mq *RabbitMQ) Close() {
	if mq.pubCh != nil {
		mq.pubCh.Close()
	}
	if mq.conn != nil {
		mq.conn.Close()
	}
}

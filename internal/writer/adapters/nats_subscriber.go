package adapters

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/domain"
	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/ports"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/sanitizer"

	"github.com/nats-io/nats.go"
)

// NATSSubscriber is the primary (driving) adapter for NATS JetStream.
// It receives messages from the broker, deserialises them into domain.Event
// values, and forwards them to the EventHandler inbound port.
// Concurrency is bounded by a semaphore of size workerCount; if all workers
// are busy the message is NAKed so JetStream redelivers it later.
type NATSSubscriber struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	sub     *nats.Subscription
	handler ports.EventHandler // inbound port — injected usecase
	sem     chan struct{}      // semaphore bounding concurrent goroutines
	wg      sync.WaitGroup     // tracks in-flight goroutines for graceful drain
	logger  *logger.Logger
}

// NewNATSSubscriber creates a durable JetStream push consumer.
// workerCount sets the maximum number of concurrently processed messages.
func NewNATSSubscriber(
	url string,
	subscribeSubject string,
	streamName string,
	consumerName string,
	maxDeliveries int,
	ackWaitSeconds int,
	workerCount int,
	tlsConfig *tls.Config,
	handler ports.EventHandler,
	log *logger.Logger,
) (*NATSSubscriber, error) {
	opts := []nats.Option{
		nats.Name("writer-subscriber"),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.ReconnectBufSize(5 * 1024 * 1024),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			if err != nil {
				log.Warn("NATS disconnected", "error", sanitizer.SanitizeError(err))
			}
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info("NATS reconnected", "url", nc.ConnectedUrl())
		}),
		nats.ErrorHandler(func(nc *nats.Conn, sub *nats.Subscription, err error) {
			subject := "<connection>"
			if sub != nil {
				subject = sub.Subject
			}
			log.Error("NATS error",
				"error", sanitizer.SanitizeError(err),
				"subject", subject)
		}),
	}

	if tlsConfig != nil {
		opts = append(opts, nats.Secure(tlsConfig))
		log.Info("TLS enabled for NATS connection")
	}

	conn, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	log.Info("Connected to NATS",
		"url", sanitizer.Sanitize(url),
		"server", conn.ConnectedUrl())

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	subscriber := &NATSSubscriber{
		conn:    conn,
		js:      js,
		handler: handler,
		sem:     make(chan struct{}, workerCount),
		logger:  log,
	}

	sub, err := js.Subscribe(
		subscribeSubject,
		subscriber.handleMessage,
		nats.Durable(consumerName),
		nats.AckExplicit(),
		nats.MaxDeliver(maxDeliveries),
		nats.AckWait(time.Duration(ackWaitSeconds)*time.Second),
		nats.BindStream(streamName),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to subscribe to JetStream stream %q: %w", streamName, err)
	}

	subscriber.sub = sub

	log.Info("Subscribed to JetStream",
		"stream", streamName,
		"subject", subscribeSubject,
		"consumer", consumerName,
		"max_deliveries", maxDeliveries,
		"ack_wait_seconds", ackWaitSeconds,
		"worker_count", workerCount)

	return subscriber, nil
}

type eventDTO struct {
	Criticality  int    `json:"criticality"`
	Timestamp    string `json:"timestamp"`
	EventMessage string `json:"eventMessage"`
}

func (s *NATSSubscriber) handleMessage(msg *nats.Msg) {
	var dto eventDTO
	if err := json.Unmarshal(msg.Data, &dto); err != nil {
		s.logger.Error("Failed to unmarshal event",
			"error", sanitizer.SanitizeError(err),
			"subject", msg.Subject,
			"data_size", len(msg.Data))
		_ = msg.Nak()
		return
	}

	event := domain.Event{
		Criticality:  dto.Criticality,
		Timestamp:    dto.Timestamp,
		EventMessage: dto.EventMessage,
	}

	s.logger.Debug("Event received from JetStream",
		"subject", msg.Subject,
		"criticality", event.Criticality,
		"timestamp", event.Timestamp)

	select {
	case s.sem <- struct{}{}:
	default:
		s.logger.Warn("Worker pool saturated, NAKing message with backoff delay",
			"criticality", event.Criticality)
		_ = msg.NakWithDelay(2 * time.Second)
		return
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.handler.Handle(ctx, event); err != nil {
			s.logger.Error("Failed to handle event",
				"error", sanitizer.SanitizeError(err),
				"criticality", event.Criticality,
				"timestamp", event.Timestamp)
			if errors.Is(err, ports.ErrInvalidEvent) {
				_ = msg.Term()
			} else {
				_ = msg.Nak()
			}
		} else {
			s.logger.Debug("Event handled successfully",
				"criticality", event.Criticality,
				"timestamp", event.Timestamp)
			_ = msg.Ack()
		}
	}()
}

func (s *NATSSubscriber) Close() error {
	s.logger.Info("Closing JetStream subscriber...")

	if s.sub != nil {
		if err := s.sub.Drain(); err != nil {
			s.logger.Error("Error draining JetStream subscription",
				"error", sanitizer.SanitizeError(err))
		}
	}

	s.wg.Wait()

	if s.conn != nil && !s.conn.IsClosed() {
		if err := s.conn.Drain(); err != nil {
			s.logger.Error("Error draining NATS connection",
				"error", sanitizer.SanitizeError(err))
		}
		s.conn.Close()
	}

	s.logger.Info("JetStream subscriber closed")
	return nil
}

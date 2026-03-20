package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/domain"

	"github.com/nats-io/nats.go"
)

type NATSPublisher struct {
	conn       *nats.Conn
	js         nats.JetStreamContext
	subject    string
	timeout    time.Duration
	maxRetries int
	retryDelay time.Duration
}

func NewNATSPublisher(conn *nats.Conn, subject, streamName string) (*NATSPublisher, error) {
	js, err := conn.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	if _, err := js.StreamInfo(streamName); err != nil {
		if _, err := js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: []string{subject},
		}); err != nil {
			return nil, fmt.Errorf("failed to create JetStream stream %q: %w", streamName, err)
		}
	}

	return &NATSPublisher{
		conn:       conn,
		js:         js,
		subject:    subject,
		timeout:    2 * time.Second,
		maxRetries: 3,
		retryDelay: 50 * time.Millisecond,
	}, nil
}

func (p *NATSPublisher) Close() error {
	if p.conn != nil {
		return p.conn.Drain()
	}
	return nil
}

type eventDTO struct {
	Criticality int    `json:"criticality"`
	Timestamp   string `json:"timestamp"` // RFC3339Nano
	Message     string `json:"eventMessage"`
}

func (p *NATSPublisher) PublishEvent(ctx context.Context, event domain.Event) error {
	dto := eventDTO{
		Criticality: event.Criticality,
		Timestamp:   event.Timestamp.Format(time.RFC3339Nano),
		Message:     event.Message,
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return err
	}

	pubCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	var lastErr error

	for range p.maxRetries {
		select {
		case <-pubCtx.Done():
			return pubCtx.Err()
		default:
		}

		if _, err := p.js.Publish(p.subject, data, nats.Context(pubCtx)); err == nil {
			return nil
		} else {
			lastErr = err
			time.Sleep(p.retryDelay)
		}
	}

	return lastErr
}

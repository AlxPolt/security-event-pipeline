package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/ports"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

const natsQueryTimeout = 10 * time.Second

type natsQueryRequest struct {
	MinCriticality int `json:"min_criticality"`
	Limit          int `json:"limit"`
}

type NATSResponder struct {
	nc      *nats.Conn
	subject string
	svc     ports.QueryHandler
	log     logger.Logger
}

type NATSResponderConfig struct {
	Subject string
}

func NewNATSResponder(nc *nats.Conn, cfg NATSResponderConfig, svc ports.QueryHandler, log logger.Logger) *NATSResponder {
	return &NATSResponder{
		nc:      nc,
		subject: cfg.Subject,
		svc:     svc,
		log:     *log.With(zap.String("adapter", "nats_responder"), zap.String("subject", cfg.Subject)),
	}
}

func (r *NATSResponder) Listen(ctx context.Context) error {
	sub, err := r.nc.Subscribe(r.subject, func(msg *nats.Msg) {
		reqCtx, cancel := context.WithTimeout(ctx, natsQueryTimeout)
		defer cancel()

		r.handleMessage(reqCtx, msg)
	})
	if err != nil {
		return fmt.Errorf("nats_responder: subscribing to %q: %w", r.subject, err)
	}
	defer func() { _ = sub.Drain() }()

	r.log.Info("nats_responder: listening for query requests")

	<-ctx.Done()
	r.log.Info("nats_responder: context cancelled, stopping")
	return nil
}

func (r *NATSResponder) handleMessage(ctx context.Context, msg *nats.Msg) {
	if msg.Reply == "" {
		r.log.Warn("received message with no reply subject, ignoring")
		return
	}

	var req natsQueryRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		r.sendError(msg.Reply, fmt.Errorf("invalid request format: %w", err))
		return
	}

	response, err := r.svc.HandleQuery(ctx, req.MinCriticality, req.Limit)
	if err != nil {
		if errors.Is(err, ports.ErrInvalidInput) {
			r.sendError(msg.Reply, err)
			return
		}
		r.log.Error("query service error", zap.Error(err))
		r.sendError(msg.Reply, fmt.Errorf("internal error"))
		return
	}

	data, err := json.Marshal(toQueryResponseDTO(response))
	if err != nil {
		r.log.Error("failed to marshal response", zap.Error(err))
		r.sendError(msg.Reply, fmt.Errorf("internal error"))
		return
	}

	if err := r.nc.Publish(msg.Reply, data); err != nil {
		r.log.Error("failed to publish response", zap.Error(err))
	}
}

type errorEnvelope struct {
	Error string `json:"error"`
}

func (r *NATSResponder) sendError(replySubject string, err error) {
	data, _ := json.Marshal(errorEnvelope{Error: err.Error()})
	if pubErr := r.nc.Publish(replySubject, data); pubErr != nil {
		r.log.Error("failed to publish error response", zap.Error(pubErr))
	}
}

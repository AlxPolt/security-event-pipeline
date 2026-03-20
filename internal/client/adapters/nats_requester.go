package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"

	"github.com/AlxPolt/sw-engineer-challenge/internal/client/domain"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

type natsQueryRequest struct {
	MinCriticality int `json:"min_criticality"`
	Limit          int `json:"limit"`
}

type NATSRequester struct {
	nc             *nats.Conn
	subject        string
	requestTimeout time.Duration
	log            *logger.Logger
}

type NATSRequesterConfig struct {
	QuerySubject   string
	RequestTimeout time.Duration
}

func NewNATSRequester(nc *nats.Conn, cfg NATSRequesterConfig, log *logger.Logger) *NATSRequester {
	timeout := cfg.RequestTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}

	return &NATSRequester{
		nc:             nc,
		subject:        cfg.QuerySubject,
		requestTimeout: timeout,
		log:            log.With(zap.String("adapter", "nats_requester")),
	}
}

func (r *NATSRequester) Request(ctx context.Context, req domain.QueryRequest) (domain.QueryResponse, error) {
	payload := natsQueryRequest{
		MinCriticality: req.MinCriticality,
		Limit:          req.Limit,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return domain.QueryResponse{}, fmt.Errorf("nats_requester: marshaling request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, r.requestTimeout)
	defer cancel()

	r.log.Debug("sending query request",
		zap.String("subject", r.subject),
		zap.Int("min_criticality", req.MinCriticality),
		zap.Int("limit", req.Limit))

	msg, err := r.nc.RequestWithContext(reqCtx, r.subject, data)
	if err != nil {
		return domain.QueryResponse{}, fmt.Errorf("nats_requester: sending request to %q: %w", r.subject, err)
	}

	var resp domain.QueryResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return domain.QueryResponse{}, fmt.Errorf("nats_requester: parsing response: %w", err)
	}

	r.log.Debug("received response",
		zap.Int("event_count", resp.Total))

	return resp, nil
}

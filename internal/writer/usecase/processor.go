package usecase

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/domain"
	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/ports"
)

type Logger interface {
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

// RetryPolicy governs how the usecase retries transient repository errors.
//
// Retry schedule with MaxAttempts=3 and BaseDelay=1s:
//
//	Attempt 1 — immediate
//	Attempt 2 — after 1 s
//	Attempt 3 — after 2 s
//
// If all attempts fail the error is returned to the caller (NATS adapter),
// which NAKs the message so JetStream redelivers it after ackWait. This
// provides an outer retry loop for prolonged infrastructure outages.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
}

var DefaultRetryPolicy = RetryPolicy{
	MaxAttempts: 3,
	BaseDelay:   time.Second,
}

// One call to Handle = one atomic event save.
// The NATS adapter ACKs only after Handle returns nil; it NAKs (redeliver)
// or Terminates (discard) on error, giving at-least-once delivery semantics.
//
// # Retry policy
// Transient repository failures are retried with exponential backoff according
// to RetryPolicy. Permanent validation errors are wrapped with ErrInvalidEvent
// and are never retried
// the NATS adapter terminates those messages immediately rather than redelivering them.
//
// # Idempotency
//
// InfluxDB v3's upsert semantics (same measurement + tag set + nanosecond
// timestamp → overwrite existing row) guarantee that a redelivered event
// produces no extra data.
type EventProcessor struct {
	repository     ports.EventRepository
	logger         Logger
	retry          RetryPolicy
	processedCount uint64
	errorCount     uint64
}

func NewEventProcessor(repo ports.EventRepository, retry RetryPolicy, log Logger) *EventProcessor {
	return &EventProcessor{
		repository: repo,
		logger:     log,
		retry:      retry,
	}
}

// sanitise → validate → save (with retry).
func (p *EventProcessor) Handle(ctx context.Context, event domain.Event) error {
	
	if err := event.Validate(); err != nil {
		atomic.AddUint64(&p.errorCount, 1)
		return fmt.Errorf("%w: %s", ports.ErrInvalidEvent, err)
	}

	event.Sanitize()


	var lastErr error
	for attempt := 1; attempt <= p.retry.MaxAttempts; attempt++ {
		if attempt > 1 {
			delay := p.retry.BaseDelay * time.Duration(1<<(attempt-2))
			p.logger.Warn("retrying repository save",
				"attempt", attempt,
				"max_attempts", p.retry.MaxAttempts,
				"backoff", delay,
				"error", lastErr)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		lastErr = p.repository.Save(ctx, event)
		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		atomic.AddUint64(&p.errorCount, 1)
		p.logger.Error("DEAD LETTER: event permanently failed to persist — replay from NATS stream",
			"dead_letter", true,
			"criticality", event.Criticality,
			"timestamp", event.Timestamp,
			"attempts", p.retry.MaxAttempts,
			"error", lastErr)
		return fmt.Errorf("failed to save event after %d attempts: %w", p.retry.MaxAttempts, lastErr)
	}

	count := atomic.AddUint64(&p.processedCount, 1)
	if count%100 == 0 {
		p.logger.Info("processing milestone",
			"processed_count", count,
			"error_count", atomic.LoadUint64(&p.errorCount))
	}

	return nil
}

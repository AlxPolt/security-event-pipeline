package usecase

import (
	"context"
	"time"

	"golang.org/x/time/rate"

	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/domain"
	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/ports"
)

type Logger interface {
	Info(msg string, keysAndValues ...any)
	Warn(msg string, keysAndValues ...any)
	Error(msg string, keysAndValues ...any)
}

type EventEmitter struct {
	generator *domain.Generator
	publisher ports.Publisher
	limiter   *rate.Limiter
	log       Logger
}

func NewEventEmitter(
	publisher ports.Publisher,
	eventsPerSecond float64,
	burst int,
	log Logger,
) *EventEmitter {
	return &EventEmitter{
		generator: domain.NewGenerator(),
		publisher: publisher,
		limiter:   rate.NewLimiter(rate.Limit(eventsPerSecond), burst),
		log:       log,
	}
}

func (e *EventEmitter) Run(ctx context.Context) {
	e.log.Info("event emitter started",
		"events_per_second", float64(e.limiter.Limit()),
		"burst", e.limiter.Burst(),
	)

	for {
		if err := e.limiter.Wait(ctx); err != nil {
			e.log.Info("event emitter stopping")
			return
		}

		event, err := e.generator.NewEvent(ctx, time.Now())
		if err != nil {
			continue
		}

		if err := e.publisher.PublishEvent(ctx, event); err != nil {
			e.log.Error("failed to publish event", "error", err)
		}
	}
}

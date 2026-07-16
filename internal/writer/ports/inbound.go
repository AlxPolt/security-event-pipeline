package ports

import (
	"context"
	"errors"

	"github.com/AlxPolt/security-event-pipeline/internal/writer/domain"
)

var ErrInvalidEvent = errors.New("invalid event")

type EventHandler interface {
	Handle(ctx context.Context, event domain.Event) error
}

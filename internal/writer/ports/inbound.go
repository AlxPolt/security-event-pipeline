package ports

import (
	"context"
	"errors"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/domain"
)

var ErrInvalidEvent = errors.New("invalid event")

type EventHandler interface {
	Handle(ctx context.Context, event domain.Event) error
}

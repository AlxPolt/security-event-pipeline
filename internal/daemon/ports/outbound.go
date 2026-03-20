package ports

import (
	"context"

	"github.com/AlxPolt/sw-engineer-challenge/internal/daemon/domain"
)

type Publisher interface {
	PublishEvent(ctx context.Context, event domain.Event) error
	Close() error
}

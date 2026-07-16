package ports

import (
	"context"

	"github.com/AlxPolt/security-event-pipeline/internal/daemon/domain"
)

type Publisher interface {
	PublishEvent(ctx context.Context, event domain.Event) error
	Close() error
}

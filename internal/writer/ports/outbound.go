package ports

import (
	"context"

	"github.com/AlxPolt/security-event-pipeline/internal/writer/domain"
)

type EventRepository interface {
	Save(ctx context.Context, event domain.Event) error
	Close() error
}

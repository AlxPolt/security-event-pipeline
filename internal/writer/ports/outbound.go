package ports

import (
	"context"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/domain"
)

type EventRepository interface {
	Save(ctx context.Context, event domain.Event) error
	Close() error
}

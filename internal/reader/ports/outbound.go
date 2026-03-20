package ports

import (
	"context"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/domain"
)

type Querier interface {
	Query(ctx context.Context, params domain.QueryParams) ([]domain.EventResult, error)
	Close() error
}

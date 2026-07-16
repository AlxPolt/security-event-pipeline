package ports

import (
	"context"
	"errors"

	"github.com/AlxPolt/security-event-pipeline/internal/reader/domain"
)

var ErrInvalidInput = errors.New("invalid input")

type QueryHandler interface {
	HandleQuery(ctx context.Context, minCriticality, limit int) (domain.QueryResponse, error)
}

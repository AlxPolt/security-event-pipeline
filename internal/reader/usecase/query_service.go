package usecase

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/domain"
	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/ports"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

// QueryService is the reader usecase.
//
// It implements ports.QueryHandler (inbound port) primary adapters call it.
// It depends on ports.Querier (outbound port) injected at startup.
// It owns input validation via domain.NewQueryParams.
type QueryService struct {
	querier ports.Querier
	log     *logger.Logger
}

func NewQueryService(querier ports.Querier, log *logger.Logger) *QueryService {
	return &QueryService{
		querier: querier,
		log:     log.With(zap.String("service", "query_service")),
	}
}

func (s *QueryService) HandleQuery(ctx context.Context, minCriticality, limit int) (domain.QueryResponse, error) {
	params, err := domain.NewQueryParams(minCriticality, limit)
	if err != nil {
		return domain.QueryResponse{}, fmt.Errorf("%w: %s", ports.ErrInvalidInput, err)
	}

	s.log.Debug("executing query",
		zap.Int("min_criticality", params.X_MinCriticality),
		zap.Int("limit", params.Y_Limit),
	)

	results, err := s.querier.Query(ctx, params)
	if err != nil {
		return domain.QueryResponse{}, fmt.Errorf("query_service: querying events: %w", err)
	}

	return domain.QueryResponse{
		Events: results,
		Total:  len(results),
	}, nil
}

package domain

import (
	"fmt"
	"time"
)

type QueryParams struct {
	X_MinCriticality int
	Y_Limit          int
}

type EventResult struct {
	Criticality  int
	Timestamp    time.Time
	EventMessage string
}

type QueryResponse struct {
	Events []EventResult
	Total  int
}

func NewQueryParams(minCriticality, limit int) (QueryParams, error) {
	if minCriticality < 1 || minCriticality > 10 {
		return QueryParams{}, fmt.Errorf("query: min_criticality %d out of range [1, 10]", minCriticality)
	}
	if limit < 1 || limit > 1000 {
		return QueryParams{}, fmt.Errorf("query: limit %d out of range [1, 1000]", limit)
	}
	return QueryParams{
		X_MinCriticality: minCriticality,
		Y_Limit:          limit,
	}, nil
}

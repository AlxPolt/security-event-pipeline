package domain

import (
	"time"
)

type Event struct {
	Criticality  int    `json:"criticality"`
	Timestamp    string `json:"timestamp"`
	EventMessage string `json:"eventMessage"`
}

type QueryRequest struct {
	MinCriticality int
	Limit          int
}

type QueryResponse struct {
	Events []EventResult `json:"events"`
	Total  int           `json:"total"`
}

type EventResult struct {
	Criticality  int       `json:"criticality"`
	Timestamp    time.Time `json:"timestamp"`
	EventMessage string    `json:"eventMessage"`
}

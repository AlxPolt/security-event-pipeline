package adapters

import (
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/domain"
)

type eventResultDTO struct {
	Criticality  int       `json:"criticality"`
	Timestamp    time.Time `json:"timestamp"`
	EventMessage string    `json:"eventMessage"`
}

type queryResponseDTO struct {
	Events []eventResultDTO `json:"events"`
	Total  int              `json:"total"`
}

func toQueryResponseDTO(r domain.QueryResponse) queryResponseDTO {
	dto := queryResponseDTO{
		Total:  r.Total,
		Events: make([]eventResultDTO, len(r.Events)),
	}
	for i, e := range r.Events {
		dto.Events[i] = eventResultDTO{
			Criticality:  e.Criticality,
			Timestamp:    e.Timestamp,
			EventMessage: e.EventMessage,
		}
	}
	return dto
}

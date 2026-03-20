package domain

import (
	"context"
	"math/rand"
	"time"
)

var securityMessages = []string{
	"Failed authentication",
	"Suspicious connection blocked",
	"Unexpected process",
}

type Event struct {
	Criticality int
	Timestamp   time.Time
	Message     string
}

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

// NewEvent creates a random security event at the given point in time.
func (g *Generator) NewEvent(ctx context.Context, now time.Time) (Event, error) {
	select {
	case <-ctx.Done():
		return Event{}, ctx.Err()
	default:
	}
	return Event{
		Criticality: rand.Intn(10) + 1,
		Timestamp:   now.UTC(),
		Message:     securityMessages[rand.Intn(len(securityMessages))],
	}, nil
}

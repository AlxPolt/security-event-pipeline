package domain

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/rand"
	"time"
)

var securityMessages = []string{
	"Failed authentication",
	"Suspicious connection blocked",
	"Unexpected process",
}

type Event struct {
	ID          string
	Criticality int
	Timestamp   time.Time
	Message     string
}

type Generator struct{}

func NewGenerator() *Generator {
	return &Generator{}
}

// NewEvent creates a random security event at the given point in time.
//
// Each event gets a unique ID assigned once, here, at creation time. The
// writer carries this ID through to InfluxDB as a tag so that two distinct
// events landing on the same nanosecond timestamp with the same severity
// don't collide under InfluxDB's point-identity dedup (measurement + tag
// set + timestamp) — see internal/writer/adapters/influx_adapter.go.
func (g *Generator) NewEvent(ctx context.Context, now time.Time) (Event, error) {
	select {
	case <-ctx.Done():
		return Event{}, ctx.Err()
	default:
	}
	return Event{
		ID:          newEventID(),
		Criticality: rand.Intn(10) + 1,
		Timestamp:   now.UTC(),
		Message:     securityMessages[rand.Intn(len(securityMessages))],
	}, nil
}

// newEventID returns a random UUID v4 (RFC 4122).
func newEventID() string {
	b := make([]byte, 16)
	_, _ = cryptorand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

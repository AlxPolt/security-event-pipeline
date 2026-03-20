package domain

import (
	"fmt"
	"strings"
	"time"
)

type Event struct {
	Criticality  int
	Timestamp    string
	EventMessage string
}

func (e Event) Validate() error {
	var errs []string

	if e.Criticality < 1 || e.Criticality > 10 {
		errs = append(errs, fmt.Sprintf("criticality %d is out of range [1-10], will be clamped", e.Criticality))
	}

	if _, err := time.Parse(time.RFC3339, e.Timestamp); err != nil {
		errs = append(errs, fmt.Sprintf("invalid timestamp format: %s", err))
	}

	if e.EventMessage == "" {
		errs = append(errs, "event message is required")
	}

	if len(e.EventMessage) > 1000 {
		errs = append(errs, "event message must be less than 1000 characters")
	}

	if len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", errs)
	}
	return nil
}

// 1       → "info"
// 2–3     → "low"
// 4–6     → "medium"
// 7–8     → "high"
// 9–10    → "critical"
func (e Event) Severity() string {
	switch {
	case e.Criticality >= 9:
		return "critical"
	case e.Criticality >= 7:
		return "high"
	case e.Criticality >= 4:
		return "medium"
	case e.Criticality >= 2:
		return "low"
	default:
		return "info"
	}
}

func (e *Event) Sanitize() {
	e.EventMessage = strings.TrimSpace(e.EventMessage)
	e.EventMessage = strings.Join(strings.Fields(e.EventMessage), " ")

	if e.Criticality < 1 {
		e.Criticality = 1
	}
	if e.Criticality > 10 {
		e.Criticality = 10
	}
}

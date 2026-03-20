package domain_test

import (
	"strings"
	"testing"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/writer/domain"
)

// Event.Validate

func TestEvent_Validate_Criticality(t *testing.T) {
	validTS := time.Now().UTC().Format(time.RFC3339)

	cases := []struct {
		criticality int
		wantErr     bool
	}{
		{0, true},
		{1, false}, // lower bound
		{5, false},
		{10, false}, // upper bound
		{11, true},
		{-1, true},
	}

	for _, tc := range cases {
		e := domain.Event{Criticality: tc.criticality, Timestamp: validTS, EventMessage: "test"}
		err := e.Validate()
		if tc.wantErr && err == nil {
			t.Errorf("criticality=%d: expected error, got nil", tc.criticality)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("criticality=%d: unexpected error: %v", tc.criticality, err)
		}
	}
}

func TestEvent_Validate_Timestamp(t *testing.T) {
	cases := []struct {
		name    string
		ts      string
		wantErr bool
	}{
		{"valid RFC3339", "2024-01-15T10:30:00Z", false},
		{"valid RFC3339 with offset", "2024-01-15T10:30:00+03:00", false},
		{"valid RFC3339Nano nanoseconds", "2024-01-15T10:30:00.123456789Z", false},
		{"valid RFC3339Nano milliseconds", "2024-01-15T10:30:00.123Z", false},
		{"empty", "", true},
		{"date only", "2024-01-15", true},
		{"garbage", "not-a-time", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := domain.Event{Criticality: 5, Timestamp: tc.ts, EventMessage: "test"}
			err := e.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("expected error for timestamp %q, got nil", tc.ts)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error for timestamp %q: %v", tc.ts, err)
			}
		})
	}
}

func TestEvent_Validate_EventMessage(t *testing.T) {
	validTS := time.Now().UTC().Format(time.RFC3339)

	t.Run("empty message rejected", func(t *testing.T) {
		e := domain.Event{Criticality: 5, Timestamp: validTS, EventMessage: ""}
		if err := e.Validate(); err == nil {
			t.Error("expected error for empty message, got nil")
		}
	})

	t.Run("message at 1000 chars accepted", func(t *testing.T) {
		e := domain.Event{Criticality: 5, Timestamp: validTS, EventMessage: strings.Repeat("a", 1000)}
		if err := e.Validate(); err != nil {
			t.Errorf("unexpected error at 1000 chars: %v", err)
		}
	})

	t.Run("message at 1001 chars rejected", func(t *testing.T) {
		e := domain.Event{Criticality: 5, Timestamp: validTS, EventMessage: strings.Repeat("a", 1001)}
		if err := e.Validate(); err == nil {
			t.Error("expected error for 1001-char message, got nil")
		}
	})
}

// Event.Sanitize

func TestEvent_Sanitize(t *testing.T) {
	cases := []struct {
		name     string
		input    domain.Event
		wantMsg  string
		wantCrit int
	}{
		{
			name:     "trims whitespace",
			input:    domain.Event{Criticality: 5, EventMessage: "  alert  "},
			wantMsg:  "alert",
			wantCrit: 5,
		},
		{
			name:     "collapses internal spaces",
			input:    domain.Event{Criticality: 5, EventMessage: "event  with   spaces"},
			wantMsg:  "event with spaces",
			wantCrit: 5,
		},
		{
			name:     "trims tabs and newlines",
			input:    domain.Event{Criticality: 5, EventMessage: "\talert\n"},
			wantMsg:  "alert",
			wantCrit: 5,
		},
		{
			name:     "criticality below 1 clamped to 1",
			input:    domain.Event{Criticality: 0, EventMessage: "test"},
			wantMsg:  "test",
			wantCrit: 1,
		},
		{
			name:     "criticality above 10 clamped to 10",
			input:    domain.Event{Criticality: 99, EventMessage: "test"},
			wantMsg:  "test",
			wantCrit: 10,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.input.Sanitize()
			if tc.input.EventMessage != tc.wantMsg {
				t.Errorf("EventMessage: got %q, want %q", tc.input.EventMessage, tc.wantMsg)
			}
			if tc.input.Criticality != tc.wantCrit {
				t.Errorf("Criticality: got %d, want %d", tc.input.Criticality, tc.wantCrit)
			}
		})
	}
}

// Sanitize + Validate pipeline

func TestSanitizeThenValidate(t *testing.T) {
	validTS := time.Now().UTC().Format(time.RFC3339)

	e := domain.Event{
		Criticality:  5,
		Timestamp:    validTS,
		EventMessage: "  multiple   spaces  ",
	}
	e.Sanitize()
	if err := e.Validate(); err != nil {
		t.Errorf("expected valid after sanitize, got: %v", err)
	}
	if e.EventMessage != "multiple spaces" {
		t.Errorf("got %q, want %q", e.EventMessage, "multiple spaces")
	}
}

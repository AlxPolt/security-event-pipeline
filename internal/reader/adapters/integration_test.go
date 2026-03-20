package adapters_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/adapters"
	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/domain"
	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/ports"
	"github.com/AlxPolt/sw-engineer-challenge/internal/reader/usecase"
	"github.com/AlxPolt/sw-engineer-challenge/pkg/logger"
)

// mockQuerier is a test double for ports.Querier.
// It replaces the real InfluxDB adapter.
type mockQuerier struct {
	events []domain.EventResult
	err    error
}

func (m *mockQuerier) Query(_ context.Context, _ domain.QueryParams) ([]domain.EventResult, error) {
	return m.events, m.err
}

func (m *mockQuerier) Close() error { return nil }

// newTestRouter wires the real HTTP stack (router → middleware → handler → usecase)
// with a mock querier. Logger is set to "error" level to keep test output clean.
func newTestRouter(t *testing.T, q ports.Querier) http.Handler {
	t.Helper()

	log, err := logger.New("test", "error")
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}

	svc := usecase.NewQueryService(q, log)
	h := adapters.NewGinHandler(svc, *log)
	return adapters.NewRouter(h, *log, "development", nil)
}

func TestGetEvents_Integration(t *testing.T) {
	ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	t.Run("returns events on valid request", func(t *testing.T) {
		q := &mockQuerier{events: []domain.EventResult{
			{Criticality: 9, Timestamp: ts, EventMessage: "SSH brute force"},
			{Criticality: 7, Timestamp: ts, EventMessage: "Port scan detected"},
		}}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?criticality=5&limit=10", nil)
		w := httptest.NewRecorder()
		newTestRouter(t, q).ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", w.Code)
		}

		var resp domain.QueryResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp.Total != 2 {
			t.Errorf("want total=2, got %d", resp.Total)
		}
		if len(resp.Events) != 2 {
			t.Errorf("want 2 events, got %d", len(resp.Events))
		}
		if resp.Events[0].Criticality != 9 {
			t.Errorf("want first event criticality=9, got %d", resp.Events[0].Criticality)
		}
	})

	t.Run("returns 400 when query parameters are missing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
		w := httptest.NewRecorder()
		newTestRouter(t, &mockQuerier{}).ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
	})

	t.Run("returns 400 when criticality is out of range", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?criticality=99&limit=10", nil)
		w := httptest.NewRecorder()
		newTestRouter(t, &mockQuerier{}).ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("want 400, got %d", w.Code)
		}
	})

	t.Run("returns 500 and does not leak error details when querier fails", func(t *testing.T) {
		q := &mockQuerier{err: errors.New("influx: connection refused")}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/events?criticality=5&limit=10", nil)
		w := httptest.NewRecorder()
		newTestRouter(t, q).ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("want 500, got %d", w.Code)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if got, _ := body["error"].(string); got != "internal server error" {
			t.Errorf("internal error detail leaked to client: %q", got)
		}
	})

	t.Run("security headers are present on every response", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		newTestRouter(t, &mockQuerier{}).ServeHTTP(w, req)

		want := map[string]string{
			"X-Content-Type-Options":  "nosniff",
			"X-Frame-Options":         "DENY",
			"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
			"Referrer-Policy":         "no-referrer",
		}
		for header, wantVal := range want {
			if got := w.Header().Get(header); got != wantVal {
				t.Errorf("header %s: want %q, got %q", header, wantVal, got)
			}
		}
	})
}

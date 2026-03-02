package worker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"uptime-lite/internal/models"
)

func target(url string) models.Target {
	return models.Target{ID: 1, URL: url, TimeoutSeconds: 5}
}

func TestCheck_StatusCodes(t *testing.T) {
	tests := []struct {
		code   int
		wantUp bool
	}{
		{http.StatusOK, true},
		{http.StatusCreated, true},
		{http.StatusAccepted, true},
		{http.StatusNoContent, true},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusNotFound, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(http.StatusText(tc.code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()

			result := check(context.Background(), target(srv.URL))

			if result.Up != tc.wantUp {
				t.Errorf("status %d: Up = %v, want %v", tc.code, result.Up, tc.wantUp)
			}
			if result.StatusCode != tc.code {
				t.Errorf("status %d: StatusCode = %d, want %d", tc.code, result.StatusCode, tc.code)
			}
		})
	}
}

func TestCheck_ServerUnreachable(t *testing.T) {
	// Port 1 is never listening under normal conditions.
	result := check(context.Background(), target("http://127.0.0.1:1"))

	if result.Up {
		t.Error("expected Up=false for unreachable server")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for unreachable server")
	}
	if result.TargetID != 1 {
		t.Errorf("TargetID = %d, want 1", result.TargetID)
	}
	if result.CheckedAt.IsZero() {
		t.Error("CheckedAt must not be zero")
	}
}

func TestCheck_PopulatesMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	before := time.Now()
	result := check(context.Background(), target(srv.URL))
	after := time.Now()

	if result.TargetID != 1 {
		t.Errorf("TargetID = %d, want 1", result.TargetID)
	}
	if result.CheckedAt.Before(before) || result.CheckedAt.After(after) {
		t.Errorf("CheckedAt %v not in expected range [%v, %v]", result.CheckedAt, before, after)
	}
	if result.LatencyMs < 0 {
		t.Errorf("LatencyMs = %d, should be non-negative", result.LatencyMs)
	}
}

func TestCheck_MeasuresLatency(t *testing.T) {
	const delay = 60 * time.Millisecond

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := check(context.Background(), target(srv.URL))

	if result.LatencyMs < delay.Milliseconds() {
		t.Errorf("LatencyMs = %d, want >= %d (server delay)", result.LatencyMs, delay.Milliseconds())
	}
}

func TestCheck_ContextCancelled(t *testing.T) {
	// Server delays to ensure the client hits the cancelled context, not a fast 200.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before the request is even made

	result := check(ctx, target(srv.URL))

	if result.Up {
		t.Error("expected Up=false when context is already cancelled")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for cancelled context")
	}
}

// TestCheck_ParentContextTimeout verifies that when the caller's context expires
// before the server responds, the probe is marked as down.
func TestCheck_ParentContextTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	tgt := models.Target{ID: 3, URL: srv.URL, TimeoutSeconds: 10}
	result := check(ctx, tgt)

	if result.Up {
		t.Error("expected Up=false when parent context expires before server responds")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for timed-out request")
	}
}

func TestCheck_InvalidURL(t *testing.T) {
	result := check(context.Background(), target("://bad url"))

	if result.Up {
		t.Error("expected Up=false for invalid URL")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error for invalid URL")
	}
}

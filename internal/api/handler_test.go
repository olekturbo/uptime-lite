package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"uptime-lite/internal/api"
	"uptime-lite/internal/models"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ── Mocks ─────────────────────────────────────────────────────────────────────

type mockStore struct {
	targets []models.Target
	results []models.CheckResult
	uptime  float64
	err     error
}

func (m *mockStore) ListTargets(_ context.Context) ([]models.Target, error) {
	return m.targets, m.err
}

func (m *mockStore) CreateTarget(_ context.Context, t models.Target) (models.Target, error) {
	if m.err != nil {
		return models.Target{}, m.err
	}
	t.ID = 42
	t.CreatedAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	return t, nil
}

func (m *mockStore) DeleteTarget(_ context.Context, _ int64) error { return m.err }

func (m *mockStore) GetHistory(_ context.Context, _ int64, _ int) ([]models.CheckResult, error) {
	return m.results, m.err
}

func (m *mockStore) GetUptimeStats(_ context.Context, _ int64, _ time.Time) (float64, error) {
	return m.uptime, m.err
}

type mockCache struct {
	status *models.TargetStatus
	err    error
}

func (m *mockCache) GetStatus(_ context.Context, _ int64) (*models.TargetStatus, error) {
	return m.status, m.err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newRouter(store api.TargetStore, cache api.StatusCache) *gin.Engine {
	return api.NewRouter(store, cache)
}

func do(t *testing.T, router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertStatus(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Errorf("status = %d, want %d (body: %s)", w.Code, want, w.Body.String())
	}
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.NewDecoder(w.Body).Decode(dst); err != nil {
		t.Fatalf("decode JSON: %v (body: %s)", err, w.Body.String())
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	r := newRouter(&mockStore{}, &mockCache{})
	w := do(t, r, http.MethodGet, "/health", "")

	assertStatus(t, w, http.StatusOK)

	var body map[string]string
	decodeJSON(t, w, &body)
	if body["status"] != "ok" {
		t.Errorf("body[status] = %q, want %q", body["status"], "ok")
	}
}

func TestListTargets(t *testing.T) {
	t.Run("returns targets", func(t *testing.T) {
		store := &mockStore{targets: []models.Target{
			{ID: 1, Name: "API", URL: "https://api.example.com"},
		}}
		w := do(t, newRouter(store, &mockCache{}), http.MethodGet, "/api/v1/targets", "")

		assertStatus(t, w, http.StatusOK)

		var targets []models.Target
		decodeJSON(t, w, &targets)
		if len(targets) != 1 {
			t.Errorf("got %d targets, want 1", len(targets))
		}
		if targets[0].Name != "API" {
			t.Errorf("targets[0].Name = %q, want %q", targets[0].Name, "API")
		}
	})

	t.Run("store error returns 500", func(t *testing.T) {
		store := &mockStore{err: errors.New("db error")}
		w := do(t, newRouter(store, &mockCache{}), http.MethodGet, "/api/v1/targets", "")
		assertStatus(t, w, http.StatusInternalServerError)
	})
}

func TestCreateTarget(t *testing.T) {
	validBody := `{"name":"My API","url":"https://example.com"}`

	t.Run("creates target and returns 201", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodPost, "/api/v1/targets", validBody)

		assertStatus(t, w, http.StatusCreated)

		var created models.Target
		decodeJSON(t, w, &created)
		if created.ID != 42 {
			t.Errorf("ID = %d, want 42", created.ID)
		}
		if created.Name != "My API" {
			t.Errorf("Name = %q, want %q", created.Name, "My API")
		}
		if created.IntervalSeconds != 60 {
			t.Errorf("IntervalSeconds = %d, want default 60", created.IntervalSeconds)
		}
		if created.TimeoutSeconds != 10 {
			t.Errorf("TimeoutSeconds = %d, want default 10", created.TimeoutSeconds)
		}
	})

	t.Run("missing name returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodPost, "/api/v1/targets",
			`{"url":"https://example.com"}`)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("invalid url returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodPost, "/api/v1/targets",
			`{"name":"x","url":"not-a-url"}`)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("malformed json returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodPost, "/api/v1/targets", `{bad}`)
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("store error returns 500", func(t *testing.T) {
		store := &mockStore{err: errors.New("insert failed")}
		w := do(t, newRouter(store, &mockCache{}), http.MethodPost, "/api/v1/targets", validBody)
		assertStatus(t, w, http.StatusInternalServerError)
	})

	t.Run("respects custom interval and timeout", func(t *testing.T) {
		body := `{"name":"x","url":"https://example.com","interval_seconds":120,"timeout_seconds":30}`
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodPost, "/api/v1/targets", body)

		assertStatus(t, w, http.StatusCreated)

		var created models.Target
		decodeJSON(t, w, &created)
		if created.IntervalSeconds != 120 {
			t.Errorf("IntervalSeconds = %d, want 120", created.IntervalSeconds)
		}
		if created.TimeoutSeconds != 30 {
			t.Errorf("TimeoutSeconds = %d, want 30", created.TimeoutSeconds)
		}
	})
}

func TestDeleteTarget(t *testing.T) {
	t.Run("success returns 204", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodDelete, "/api/v1/targets/1", "")
		assertStatus(t, w, http.StatusNoContent)
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodDelete, "/api/v1/targets/abc", "")
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("store error returns 500", func(t *testing.T) {
		store := &mockStore{err: errors.New("not found")}
		w := do(t, newRouter(store, &mockCache{}), http.MethodDelete, "/api/v1/targets/99", "")
		assertStatus(t, w, http.StatusInternalServerError)
	})
}

func TestGetStatus(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	t.Run("cached status returns 200", func(t *testing.T) {
		cache := &mockCache{status: &models.TargetStatus{
			TargetID: 1, Up: true, StatusCode: 200, LatencyMs: 42, CheckedAt: now,
		}}
		w := do(t, newRouter(&mockStore{}, cache), http.MethodGet, "/api/v1/targets/1/status", "")

		assertStatus(t, w, http.StatusOK)

		var s models.TargetStatus
		decodeJSON(t, w, &s)
		if !s.Up {
			t.Error("Up = false, want true")
		}
		if s.LatencyMs != 42 {
			t.Errorf("LatencyMs = %d, want 42", s.LatencyMs)
		}
	})

	t.Run("no cached status returns 404", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{status: nil}), http.MethodGet, "/api/v1/targets/1/status", "")
		assertStatus(t, w, http.StatusNotFound)
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodGet, "/api/v1/targets/xyz/status", "")
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("cache error returns 500", func(t *testing.T) {
		cache := &mockCache{err: errors.New("redis down")}
		w := do(t, newRouter(&mockStore{}, cache), http.MethodGet, "/api/v1/targets/1/status", "")
		assertStatus(t, w, http.StatusInternalServerError)
	})
}

func TestGetHistory(t *testing.T) {
	t.Run("returns check results", func(t *testing.T) {
		store := &mockStore{results: []models.CheckResult{
			{ID: 1, TargetID: 7, Up: true, StatusCode: 200},
			{ID: 2, TargetID: 7, Up: false, StatusCode: 500},
		}}
		w := do(t, newRouter(store, &mockCache{}), http.MethodGet, "/api/v1/targets/7/history", "")

		assertStatus(t, w, http.StatusOK)

		var results []models.CheckResult
		decodeJSON(t, w, &results)
		if len(results) != 2 {
			t.Errorf("got %d results, want 2", len(results))
		}
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodGet, "/api/v1/targets/nope/history", "")
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("store error returns 500", func(t *testing.T) {
		store := &mockStore{err: errors.New("query failed")}
		w := do(t, newRouter(store, &mockCache{}), http.MethodGet, "/api/v1/targets/1/history", "")
		assertStatus(t, w, http.StatusInternalServerError)
	})
}

func TestGetStats(t *testing.T) {
	t.Run("returns uptime percentage", func(t *testing.T) {
		store := &mockStore{uptime: 98.5}
		w := do(t, newRouter(store, &mockCache{}), http.MethodGet, "/api/v1/targets/1/stats", "")

		assertStatus(t, w, http.StatusOK)

		var body map[string]any
		decodeJSON(t, w, &body)

		if body["uptime_pct"] != 98.5 {
			t.Errorf("uptime_pct = %v, want 98.5", body["uptime_pct"])
		}
		if int(body["period_hours"].(float64)) != 24 {
			t.Errorf("period_hours = %v, want 24", body["period_hours"])
		}
		if int(body["target_id"].(float64)) != 1 {
			t.Errorf("target_id = %v, want 1", body["target_id"])
		}
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		w := do(t, newRouter(&mockStore{}, &mockCache{}), http.MethodGet, "/api/v1/targets/bad/stats", "")
		assertStatus(t, w, http.StatusBadRequest)
	})

	t.Run("store error returns 500", func(t *testing.T) {
		store := &mockStore{err: errors.New("stats query failed")}
		w := do(t, newRouter(store, &mockCache{}), http.MethodGet, "/api/v1/targets/1/stats", "")
		assertStatus(t, w, http.StatusInternalServerError)
	})
}

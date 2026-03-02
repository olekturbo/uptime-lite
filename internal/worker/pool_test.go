package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	"uptime-lite/internal/models"
)

// ── Mocks ─────────────────────────────────────────────────────────────────────

type mockRepo struct {
	targets []models.Target
	err     error
	saved   []models.CheckResult
}

func (m *mockRepo) ListActiveTargets(_ context.Context) ([]models.Target, error) {
	return m.targets, m.err
}

func (m *mockRepo) SaveResult(_ context.Context, r models.CheckResult) error {
	m.saved = append(m.saved, r)
	return m.err
}

type mockSetter struct {
	err      error
	statuses []models.TargetStatus
}

func (m *mockSetter) SetStatus(_ context.Context, s models.TargetStatus) error {
	m.statuses = append(m.statuses, s)
	return m.err
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func newTestPool(repo targetRepo, cache statusSetter) *Pool {
	return &Pool{
		repo:  repo,
		cache: cache,
		jobs:  make(chan models.Target, 16),
	}
}

// ── dispatch tests ────────────────────────────────────────────────────────────

func TestDispatch_SendsDueTargets(t *testing.T) {
	repo := &mockRepo{
		targets: []models.Target{
			{ID: 1, URL: "http://a.test", IntervalSeconds: 60},
			{ID: 2, URL: "http://b.test", IntervalSeconds: 60},
		},
	}
	p := newTestPool(repo, &mockSetter{})

	lastChecked := map[int64]time.Time{
		// Both targets were last checked 2 minutes ago → past their 60s interval.
		1: time.Now().Add(-2 * time.Minute),
		2: time.Now().Add(-2 * time.Minute),
	}

	p.dispatch(context.Background(), lastChecked)

	if got := len(p.jobs); got != 2 {
		t.Errorf("jobs channel length = %d, want 2", got)
	}
}

func TestDispatch_SkipsTargetsNotYetDue(t *testing.T) {
	repo := &mockRepo{
		targets: []models.Target{
			{ID: 1, URL: "http://a.test", IntervalSeconds: 300},
		},
	}
	p := newTestPool(repo, &mockSetter{})

	lastChecked := map[int64]time.Time{
		// Checked 10 seconds ago, interval is 300s → not due yet.
		1: time.Now().Add(-10 * time.Second),
	}

	p.dispatch(context.Background(), lastChecked)

	if got := len(p.jobs); got != 0 {
		t.Errorf("jobs channel length = %d, want 0 (target not due)", got)
	}
}

func TestDispatch_NewTargetAlwaysDue(t *testing.T) {
	// A target with no entry in lastChecked (zero time) should always be dispatched.
	repo := &mockRepo{
		targets: []models.Target{
			{ID: 99, URL: "http://new.test", IntervalSeconds: 60},
		},
	}
	p := newTestPool(repo, &mockSetter{})

	p.dispatch(context.Background(), map[int64]time.Time{})

	if got := len(p.jobs); got != 1 {
		t.Errorf("jobs channel length = %d, want 1 (new target always due)", got)
	}
}

func TestDispatch_UpdatesLastChecked(t *testing.T) {
	repo := &mockRepo{
		targets: []models.Target{
			{ID: 5, URL: "http://c.test", IntervalSeconds: 60},
		},
	}
	p := newTestPool(repo, &mockSetter{})
	lastChecked := map[int64]time.Time{}

	before := time.Now()
	p.dispatch(context.Background(), lastChecked)

	if lastChecked[5].Before(before) {
		t.Errorf("lastChecked[5] = %v, expected >= %v after dispatch", lastChecked[5], before)
	}
}

func TestDispatch_FullChannelDoesNotBlock(t *testing.T) {
	// Fill the channel to capacity, then call dispatch — must return without blocking.
	repo := &mockRepo{
		targets: []models.Target{
			{ID: 1, URL: "http://a.test", IntervalSeconds: 60},
		},
	}
	p := &Pool{
		repo:  repo,
		cache: &mockSetter{},
		jobs:  make(chan models.Target, 1), // capacity 1
	}
	// Pre-fill the channel.
	p.jobs <- models.Target{}

	done := make(chan struct{})
	go func() {
		p.dispatch(context.Background(), map[int64]time.Time{})
		close(done)
	}()

	select {
	case <-done:
		// pass — dispatch returned without blocking
	case <-time.After(500 * time.Millisecond):
		t.Error("dispatch blocked on a full jobs channel")
	}
}

func TestDispatch_StoreErrorIsHandledGracefully(t *testing.T) {
	repo := &mockRepo{err: errors.New("db unavailable")}
	p := newTestPool(repo, &mockSetter{})

	// Must not panic and must not enqueue anything.
	p.dispatch(context.Background(), map[int64]time.Time{})

	if got := len(p.jobs); got != 0 {
		t.Errorf("jobs channel length = %d, want 0 after store error", got)
	}
}

func TestDispatch_MixedDueAndNotDue(t *testing.T) {
	repo := &mockRepo{
		targets: []models.Target{
			{ID: 1, IntervalSeconds: 60},  // last checked 2 min ago → due
			{ID: 2, IntervalSeconds: 300}, // last checked 10 s ago  → not due
			{ID: 3, IntervalSeconds: 60},  // never checked          → due
		},
	}
	p := newTestPool(repo, &mockSetter{})

	lastChecked := map[int64]time.Time{
		1: time.Now().Add(-2 * time.Minute),
		2: time.Now().Add(-10 * time.Second),
	}

	p.dispatch(context.Background(), lastChecked)

	if got := len(p.jobs); got != 2 {
		t.Errorf("jobs channel length = %d, want 2 (targets 1 and 3)", got)
	}
}

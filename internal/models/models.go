package models

import "time"

// Target is a monitored HTTP endpoint.
type Target struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	IntervalSeconds int       `json:"interval_seconds"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	Active          bool      `json:"active"`
	CreatedAt       time.Time `json:"created_at"`
}

// CheckResult represents a single uptime probe outcome.
type CheckResult struct {
	ID         int64     `json:"id"`
	TargetID   int64     `json:"target_id"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
	Up         bool      `json:"up"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// TargetStatus is the latest probe result, cached in Redis.
type TargetStatus struct {
	TargetID   int64     `json:"target_id"`
	Up         bool      `json:"up"`
	StatusCode int       `json:"status_code"`
	LatencyMs  int64     `json:"latency_ms"`
	CheckedAt  time.Time `json:"checked_at"`
}

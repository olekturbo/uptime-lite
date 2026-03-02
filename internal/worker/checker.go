package worker

import (
	"context"
	"net/http"
	"time"

	"uptime-lite/internal/models"
)

// check performs a single HTTP GET probe against a target and returns the result.
// The probe respects both the parent context (for graceful shutdown) and a
// per-target timeout derived from target.TimeoutSeconds.
func check(ctx context.Context, target models.Target) models.CheckResult {
	result := models.CheckResult{
		TargetID:  target.ID,
		CheckedAt: time.Now().UTC(),
	}

	timeout := time.Duration(target.TimeoutSeconds) * time.Second
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, target.URL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	// Use the shared default transport so TCP connections are pooled across checks.
	client := &http.Client{
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	start := time.Now()
	resp, err := client.Do(req)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Up = false
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.Up = resp.StatusCode >= 200 && resp.StatusCode < 400
	return result
}

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"uptime-lite/internal/models"
)

// Repository handles all database interactions.
type Repository struct {
	db *pgxpool.Pool
}

// New creates a Repository backed by the given pool.
func New(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// --- Targets ---

func (r *Repository) CreateTarget(ctx context.Context, t models.Target) (models.Target, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO targets (name, url, interval_seconds, timeout_seconds, active)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		t.Name, t.URL, t.IntervalSeconds, t.TimeoutSeconds, t.Active,
	).Scan(&t.ID, &t.CreatedAt)
	if err != nil {
		return models.Target{}, fmt.Errorf("create target: %w", err)
	}
	return t, nil
}

func (r *Repository) ListTargets(ctx context.Context) ([]models.Target, error) {
	return r.queryTargets(ctx, `
		SELECT id, name, url, interval_seconds, timeout_seconds, active, created_at
		FROM targets ORDER BY id`)
}

func (r *Repository) ListActiveTargets(ctx context.Context) ([]models.Target, error) {
	return r.queryTargets(ctx, `
		SELECT id, name, url, interval_seconds, timeout_seconds, active, created_at
		FROM targets WHERE active = TRUE ORDER BY id`)
}

func (r *Repository) DeleteTarget(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM targets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete target: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("target %d not found", id)
	}
	return nil
}

func (r *Repository) queryTargets(ctx context.Context, query string, args ...any) ([]models.Target, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query targets: %w", err)
	}
	defer rows.Close()

	var targets []models.Target
	for rows.Next() {
		var t models.Target
		if err := rows.Scan(&t.ID, &t.Name, &t.URL, &t.IntervalSeconds, &t.TimeoutSeconds, &t.Active, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan target: %w", err)
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// --- Check Results ---

func (r *Repository) SaveResult(ctx context.Context, res models.CheckResult) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO check_results (target_id, status_code, latency_ms, up, error, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		res.TargetID, res.StatusCode, res.LatencyMs, res.Up, res.Error, res.CheckedAt,
	)
	if err != nil {
		return fmt.Errorf("save result: %w", err)
	}
	return nil
}

func (r *Repository) GetHistory(ctx context.Context, targetID int64, limit int) ([]models.CheckResult, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, target_id, status_code, latency_ms, up, error, checked_at
		FROM check_results
		WHERE target_id = $1
		ORDER BY checked_at DESC LIMIT $2`,
		targetID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	defer rows.Close()

	var results []models.CheckResult
	for rows.Next() {
		var cr models.CheckResult
		if err := rows.Scan(&cr.ID, &cr.TargetID, &cr.StatusCode, &cr.LatencyMs, &cr.Up, &cr.Error, &cr.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan result: %w", err)
		}
		results = append(results, cr)
	}
	return results, rows.Err()
}

// GetUptimeStats returns the percentage of successful checks since the given time.
func (r *Repository) GetUptimeStats(ctx context.Context, targetID int64, since time.Time) (float64, error) {
	var total, up int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN up THEN 1 ELSE 0 END), 0)
		FROM check_results
		WHERE target_id = $1 AND checked_at > $2`,
		targetID, since,
	).Scan(&total, &up)
	if err != nil {
		return 0, fmt.Errorf("get uptime stats: %w", err)
	}
	if total == 0 {
		return 0, nil
	}
	return float64(up) / float64(total) * 100, nil
}

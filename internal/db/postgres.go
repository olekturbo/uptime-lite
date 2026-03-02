package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect opens and validates a pgx connection pool.
func Connect(dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return pool, nil
}

// Migrate runs idempotent DDL statements to set up the schema.
func Migrate(pool *pgxpool.Pool) error {
	_, err := pool.Exec(context.Background(), schema)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS targets (
	id               BIGSERIAL PRIMARY KEY,
	name             VARCHAR(255) NOT NULL,
	url              TEXT         NOT NULL,
	interval_seconds INT          NOT NULL DEFAULT 60,
	timeout_seconds  INT          NOT NULL DEFAULT 10,
	active           BOOLEAN      NOT NULL DEFAULT TRUE,
	created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS check_results (
	id          BIGSERIAL   PRIMARY KEY,
	target_id   BIGINT      NOT NULL REFERENCES targets(id) ON DELETE CASCADE,
	status_code INT         NOT NULL DEFAULT 0,
	latency_ms  BIGINT      NOT NULL DEFAULT 0,
	up          BOOLEAN     NOT NULL,
	error       TEXT        NOT NULL DEFAULT '',
	checked_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_check_results_target_id  ON check_results(target_id);
CREATE INDEX IF NOT EXISTS idx_check_results_checked_at ON check_results(checked_at DESC);
`

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Job struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Status       string          `json:"status"`
	Retries      int             `json:"retries"`
	Attempts     int             `json:"attempts"`
	Payload      json.RawMessage `json:"payload"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(ctx context.Context, databaseURL string) (*Postgres, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("pgx pool: %w", err)
	}
	s := &Postgres{pool: pool}
	if err := s.Migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

func (s *Postgres) Close() {
	s.pool.Close()
}

func (s *Postgres) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS jobs (
	id UUID PRIMARY KEY,
	type TEXT NOT NULL,
	status TEXT NOT NULL,
	retries INT NOT NULL DEFAULT 0,
	attempts INT NOT NULL DEFAULT 0,
	payload JSONB NOT NULL DEFAULT '{}'::jsonb,
	error_message TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS jobs_created_at_idx ON jobs (created_at DESC);
`)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	return nil
}

func (s *Postgres) CreateJob(ctx context.Context, id, jobType string, retries int, payload map[string]interface{}) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("payload json: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
INSERT INTO jobs (id, type, status, retries, attempts, payload)
VALUES ($1::uuid, $2, 'pending', $3, 0, $4::jsonb)
`, id, jobType, retries, b)
	if err != nil {
		return fmt.Errorf("insert job: %w", err)
	}
	return nil
}

func (s *Postgres) MarkProcessing(ctx context.Context, jobID string) error {
	_, err := s.pool.Exec(ctx, `
UPDATE jobs SET status = 'processing', updated_at = now() WHERE id = $1::uuid
`, jobID)
	if err != nil {
		return fmt.Errorf("mark processing: %w", err)
	}
	return nil
}

func (s *Postgres) SetJobPendingRetry(ctx context.Context, jobID string, attempts int, lastErr string) error {
	var errMsg *string
	if lastErr != "" {
		errMsg = &lastErr
	}
	_, err := s.pool.Exec(ctx, `
UPDATE jobs
SET status = 'pending', attempts = $2, error_message = $3, updated_at = now()
WHERE id = $1::uuid
`, jobID, attempts, errMsg)
	if err != nil {
		return fmt.Errorf("pending retry: %w", err)
	}
	return nil
}

func (s *Postgres) CompleteJob(ctx context.Context, jobID string, attempts int) error {
	_, err := s.pool.Exec(ctx, `
UPDATE jobs SET status = 'completed', attempts = $2, error_message = NULL, updated_at = now()
WHERE id = $1::uuid
`, jobID, attempts)
	if err != nil {
		return fmt.Errorf("complete job: %w", err)
	}
	return nil
}

func (s *Postgres) FailJob(ctx context.Context, jobID string, attempts int, errMsg string) error {
	_, err := s.pool.Exec(ctx, `
UPDATE jobs SET status = 'failed', attempts = $2, error_message = $3, updated_at = now()
WHERE id = $1::uuid
`, jobID, attempts, errMsg)
	if err != nil {
		return fmt.Errorf("fail job: %w", err)
	}
	return nil
}

func (s *Postgres) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := s.pool.Query(ctx, `
SELECT id, type, status, retries, attempts, payload, error_message, created_at, updated_at
FROM jobs
ORDER BY created_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	defer rows.Close()

	var out []Job
	for rows.Next() {
		var j Job
		if err := rows.Scan(
			&j.ID, &j.Type, &j.Status, &j.Retries, &j.Attempts,
			&j.Payload, &j.ErrorMessage, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan job: %w", err)
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

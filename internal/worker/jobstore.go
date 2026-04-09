package worker

import "context"

// JobStore updates Postgres (or other persistence) when jobs move through the pipeline.
type JobStore interface {
	MarkProcessing(ctx context.Context, jobID string) error
	SetJobPendingRetry(ctx context.Context, jobID string, attempts int, lastErr string) error
	CompleteJob(ctx context.Context, jobID string, attempts int) error
	FailJob(ctx context.Context, jobID string, attempts int, errMsg string) error
}

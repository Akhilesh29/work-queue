package producer

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"workqueue/internal/queue"
	"workqueue/internal/store"
	"workqueue/internal/task"
)

type Handler struct {
	Redis     *redis.Client
	QueueName string
	Store     *store.Postgres
}

func (h Handler) EnqueueTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req task.Task
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Type == "" {
		http.Error(w, "type is required", http.StatusBadRequest)
		return
	}
	if req.Retries < 0 {
		http.Error(w, "retries cannot be negative", http.StatusBadRequest)
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]interface{}{}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var jobID string
	if h.Store != nil {
		jobID = uuid.New().String()
		if err := h.Store.CreateJob(ctx, jobID, req.Type, req.Retries, req.Payload); err != nil {
			http.Error(w, "failed to persist job", http.StatusInternalServerError)
			return
		}
		req.JobID = jobID
	}

	if err := queue.Enqueue(ctx, h.Redis, h.QueueName, req); err != nil {
		http.Error(w, "failed to enqueue task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"status":     "queued",
		"type":       req.Type,
		"retries":    req.Retries,
		"queue_name": h.QueueName,
	}
	if jobID != "" {
		resp["job_id"] = jobID
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Store == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}
	limit := 50
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	jobs, err := h.Store.ListJobs(ctx, limit)
	if err != nil {
		http.Error(w, "failed to list jobs", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}

package producer

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"workqueue/internal/queue"
	"workqueue/internal/task"
)

type Handler struct {
	Redis     *redis.Client
	QueueName string
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

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := queue.Enqueue(ctx, h.Redis, h.QueueName, req); err != nil {
		http.Error(w, "failed to enqueue task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "queued",
		"type":       req.Type,
		"retries":    req.Retries,
		"queue_name": h.QueueName,
	})
}


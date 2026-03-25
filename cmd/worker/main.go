package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"workqueue/internal/queue"
	"workqueue/internal/worker"
)

func main() {
	redisClient := queue.NewRedisClient()
	defer func() { _ = redisClient.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed connecting redis: %v", err)
	}

	queueName := os.Getenv("QUEUE_NAME")
	if queueName == "" {
		queueName = queue.DefaultQueueName
	}

	concurrency := 2
	if c := os.Getenv("WORKER_CONCURRENCY"); c != "" {
		if parsed, err := strconv.Atoi(c); err == nil && parsed > 0 {
			concurrency = parsed
		}
	}

	metrics := &worker.Metrics{}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	wg := worker.StartConsumers(workerCtx, redisClient, queueName, concurrency, metrics)

	port := os.Getenv("WORKER_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		qlen, err := redisClient.LLen(r.Context(), queueName).Result()
		if err != nil {
			http.Error(w, "failed to read queue length", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_jobs_in_queue": qlen,
			"jobs_done":           metrics.JobsDone.Load(),
			"jobs_failed":         metrics.JobsFailed.Load(),
			"worker_concurrency":  concurrency,
			"queue_name":          queueName,
		})
	})

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("worker metrics listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("worker server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	workerCancel()
	wg.Wait()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("worker shutdown error: %v", err)
	}
}


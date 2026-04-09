package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"workqueue/internal/middleware"
	"workqueue/internal/producer"
	"workqueue/internal/queue"
	"workqueue/internal/store"
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

	var pgStore *store.Postgres
	if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
		s, err := store.NewPostgres(context.Background(), dsn)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		defer s.Close()
		pgStore = s
		log.Println("postgres: connected, jobs API enabled")
	}

	// Optional: run worker consumers inside the same service.
	// This makes single-service deployments possible on platforms where creating
	// multiple services is inconvenient. Set ENABLE_WORKER=false to disable.
	enableWorker := os.Getenv("ENABLE_WORKER") != "false"
	concurrency := 2
	if c := os.Getenv("WORKER_CONCURRENCY"); c != "" {
		if parsed, err := strconv.Atoi(c); err == nil && parsed > 0 {
			concurrency = parsed
		}
	}
	metrics := &worker.Metrics{}
	workerCtx, workerCancel := context.WithCancel(context.Background())
	var workerWG *sync.WaitGroup
	if enableWorker {
		log.Printf("worker: enabled (concurrency=%d)", concurrency)
		workerWG = worker.StartConsumers(workerCtx, redisClient, queueName, concurrency, metrics, pgStore)
	} else {
		log.Printf("worker: disabled (ENABLE_WORKER=false)")
	}

	h := producer.Handler{Redis: redisClient, QueueName: queueName, Store: pgStore}
	mux := http.NewServeMux()
	mux.HandleFunc("/enqueue", h.EnqueueTask)
	mux.HandleFunc("/api/jobs", h.ListJobs)
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
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	port := os.Getenv("PRODUCER_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           middleware.CORSMiddleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("producer listening on :%s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("producer server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	workerCancel()
	if workerWG != nil {
		workerWG.Wait()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("producer shutdown error: %v", err)
	}
}

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"workqueue/internal/middleware"
	"workqueue/internal/producer"
	"workqueue/internal/queue"
	"workqueue/internal/store"
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

	h := producer.Handler{Redis: redisClient, QueueName: queueName, Store: pgStore}
	mux := http.NewServeMux()
	mux.HandleFunc("/enqueue", h.EnqueueTask)
	mux.HandleFunc("/api/jobs", h.ListJobs)
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

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("producer shutdown error: %v", err)
	}
}

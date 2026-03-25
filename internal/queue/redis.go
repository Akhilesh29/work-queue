package queue

import (
	"context"
	"encoding/json"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"
	"workqueue/internal/task"
)

const DefaultQueueName = "workqueue:jobs"

func NewRedisClient() *redis.Client {
	// Prefer a single connection string (works well with managed Redis like Upstash).
	// Example: REDIS_URL="rediss://:password@host:port"
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			// Fall back to basic options instead of crashing the whole service.
			// Errors will surface via client.Ping().
		} else {
			return redis.NewClient(opt)
		}
	}

	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	password := os.Getenv("REDIS_PASSWORD")
	username := os.Getenv("REDIS_USERNAME")

	useTLS := false
	if v := os.Getenv("REDIS_TLS"); v != "" {
		parsed, err := strconv.ParseBool(v)
		useTLS = err == nil && parsed
	}

	var tlsConfig *tls.Config
	if useTLS {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return redis.NewClient(&redis.Options{
		Addr: addr,
		Password: password,
		Username: username,
		TLSConfig: tlsConfig,
	})
}

func Enqueue(ctx context.Context, client *redis.Client, queueName string, t task.Task) error {
	body, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("marshal task: %w", err)
	}
	if err := client.RPush(ctx, queueName, body).Err(); err != nil {
		return fmt.Errorf("enqueue task: %w", err)
	}
	return nil
}


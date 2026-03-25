package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"workqueue/internal/queue"
	"workqueue/internal/task"
)

type Metrics struct {
	JobsDone   atomic.Int64
	JobsFailed atomic.Int64
}

func StartConsumers(ctx context.Context, redisClient *redis.Client, queueName string, concurrency int, metrics *Metrics) *sync.WaitGroup {
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			runConsumer(ctx, redisClient, queueName, workerID, metrics)
		}(i + 1)
	}
	return &wg
}

func runConsumer(ctx context.Context, redisClient *redis.Client, queueName string, workerID int, metrics *Metrics) {
	for {
		select {
		case <-ctx.Done():
			log.Printf("[worker-%d] shutting down", workerID)
			return
		default:
		}

		result, err := redisClient.BRPop(ctx, 5*time.Second, queueName).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			log.Printf("[worker-%d] pop error: %v", workerID, err)
			time.Sleep(1 * time.Second)
			continue
		}
		if len(result) < 2 {
			continue
		}

		var t task.Task
		if err := json.Unmarshal([]byte(result[1]), &t); err != nil {
			log.Printf("[worker-%d] invalid task payload: %v", workerID, err)
			metrics.JobsFailed.Add(1)
			continue
		}

		if err := ProcessTask(t); err != nil {
			t.Attempts++
			if t.Attempts <= t.Retries {
				log.Printf("[worker-%d] retrying task type=%s attempt=%d/%d", workerID, t.Type, t.Attempts, t.Retries)
				if enqueueErr := queue.Enqueue(ctx, redisClient, queueName, t); enqueueErr != nil {
					log.Printf("[worker-%d] failed requeue task: %v", workerID, enqueueErr)
					metrics.JobsFailed.Add(1)
				}
				continue
			}
			log.Printf("[worker-%d] task failed after retries type=%s err=%v", workerID, t.Type, err)
			metrics.JobsFailed.Add(1)
			continue
		}

		metrics.JobsDone.Add(1)
		log.Printf("[worker-%d] task completed type=%s", workerID, t.Type)
	}
}

func ProcessTask(taskToExecute task.Task) error {
	if taskToExecute.Type == "" {
		return fmt.Errorf("task type is empty")
	}
	if taskToExecute.Payload == nil {
		return fmt.Errorf("payload is empty")
	}

	switch taskToExecute.Type {
	case "send_email":
		time.Sleep(2 * time.Second)
		log.Printf("sending email to=%v subject=%v", taskToExecute.Payload["to"], taskToExecute.Payload["subject"])
		return nil
	case "resize_image":
		log.Printf("resizing image x=%v y=%v", taskToExecute.Payload["new_x"], taskToExecute.Payload["new_y"])
		return nil
	case "generate_pdf":
		log.Printf("generating pdf with payload=%v", taskToExecute.Payload)
		return nil
	default:
		return fmt.Errorf("unsupported task type: %s", taskToExecute.Type)
	}
}


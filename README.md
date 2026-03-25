# WorkQueue

A distributed background task processing system written in Go, using Redis for job queuing.

## Services

- **Producer**: accepts new tasks via HTTP `POST /enqueue`.
- **Worker**: consumes queued tasks and executes them concurrently.
- **Redis**: backing queue store.

## Task format

```json
{
  "type": "send_email",
  "retries": 3,
  "payload": {
    "to": "someone@example.com",
    "subject": "Welcome!"
  }
}
```

Notes:
- `type` is required and controls which handler runs in the worker (`send_email`, `resize_image`, `generate_pdf`).
- `payload` is a free-form key/value object (any fields you want).
- `retries` controls how many times the worker will retry on failure.
- `attempts` is internal (the worker tracks it automatically).

**High level overview**

```mermaid
flowchart LR
  A[Client / App] -->|POST /enqueue| P[Producer]
  P -->|RPUSH job(JSON)| Q[(Redis Queue)]
  Q -->|BRPOP job| W[Worker (concurrency)]
  W --> E{ProcessTask(type)}

  E -->|success| M[metrics: jobs_done++]
  E -->|failure & attempts <= retries| Q
  E -->|failure & attempts > retries| F[metrics: jobs_failed++]
```

## Run locally

## Prerequisites

- `Redis` (either installed locally or via Docker)
- `Go` v1.22+ (only if you run with `go run`)
- `Docker` + `docker compose` (only if you run via `docker compose`)

### 1) Start Redis

If you already have Redis locally:

```bash
redis-server
```

Or use Docker:

```bash
docker run --rm -p 6379:6379 redis:7-alpine
```

### 2) Start producer

```bash
go run ./cmd/producer
```

Producer runs on `http://localhost:8080`.

### 3) Start worker

In another terminal:

```bash
go run ./cmd/worker
```

Worker metrics run on `http://localhost:8081/metrics`.

## Docker compose

Start everything with:

```bash
docker compose up --build
```

If you prefer background mode:

```bash
docker compose up --build -d
```

## Deploy to Render (free-tier friendly)

This system has 2 services, so on Render you deploy:

- a **Producer** web service (runs `cmd/producer` on port `8080`)
- a **Worker** web service (runs `cmd/worker` on port `8081`)

You also need a hosted Redis (because Render services must connect to an external Redis). A common free-tier option is a managed Redis provider like Upstash.

### 1) Push code to GitHub

Make sure the repo is publicly accessible or your Render service has access.

### 2) Create Producer service

On Render: `New + Web Service` -> `From Docker`.

- Dockerfile: `Dockerfile.producer`
- Internal port: `8080`
- Environment variables:
  - `REDIS_URL` (required)
  - `QUEUE_NAME` (optional, defaults to `workqueue:jobs`)

### 3) Create Worker service

On Render: `New + Web Service` -> `From Docker`.

- Dockerfile: `Dockerfile.worker`
- Internal port: `8081`
- Environment variables:
  - `REDIS_URL` (required)
  - `QUEUE_NAME` (optional)
  - `WORKER_CONCURRENCY` (optional)

### 4) Test

Enqueue:

```bash
curl -X POST https://<producer-render-url>/enqueue \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"send_email\",\"retries\":3,\"payload\":{\"to\":\"user@example.com\",\"subject\":\"hello\"}}"
```

Then check:

```bash
curl https://<worker-render-url>/metrics
```

## Endpoints

### Producer

- `POST /enqueue`
- `GET /health`

### Worker

- `GET /metrics`
- `GET /health`

## Sample requests

Enqueue:

```bash
curl -X POST http://localhost:8080/enqueue \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"send_email\",\"retries\":3,\"payload\":{\"to\":\"user@example.com\",\"subject\":\"hello\"}}"
```

Fetch metrics:

```bash
curl http://localhost:8081/metrics
```

## API Responses (examples)

### Producer: `POST /enqueue`

Success (`200 OK`):

```json
{
  "status": "queued",
  "type": "send_email",
  "retries": 3,
  "queue_name": "workqueue:jobs"
}
```

Common error responses:

- `400 Bad Request` (invalid JSON body):
  - Response body: `invalid JSON body`
- `400 Bad Request` (`type` missing/empty):
  - Response body: `type is required`
- `400 Bad Request` (`retries` negative):
  - Response body: `retries cannot be negative`
- `500 Internal Server Error` (Redis enqueue failed):
  - Response body: `failed to enqueue task`

### Worker: `GET /metrics`

Response (`200 OK`):

```json
{
  "total_jobs_in_queue": 0,
  "jobs_done": 1,
  "jobs_failed": 0,
  "worker_concurrency": 3,
  "queue_name": "workqueue:jobs"
}
```

### `/health`

- `200 OK` with body: `ok`

### Quick PowerShell demo

This repository includes `scripts/demo.ps1` which will:

1. Start the stack with Docker compose
2. Wait for `/health`
3. Enqueue a sample `send_email` task
4. Poll `/metrics` so you can show progress in a video

Run it from the repo root:

```powershell
.\scripts\demo.ps1
```

## Built-in task types

The worker currently supports:

- `send_email`
- `resize_image`
- `generate_pdf`

Add new task types in `internal/worker/worker.go` inside `ProcessTask`.

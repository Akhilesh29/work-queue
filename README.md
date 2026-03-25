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
flowchart TB
  %% End-to-end pipeline
  C["Client<br/>HTTP client"] -->|POST /enqueue| P["Producer<br/>Go HTTP :8080"]
  P -->|RPUSH job JSON| Q["Redis<br/>Queue :6379"]
  Q -->|BRPOP job| W["Worker<br/>Goroutines :8081"]

  %% Task processing
  W --> D{ProcessTask}
  D -->|success| DONE["jobs_done<br/>metrics updated"]
  D -->|failure - retry| REQ["Re-queued<br/>RPUSH back to Redis"] --> Q
  D -->|failure - retries exhausted| FAIL["jobs_failed<br/>metrics updated"]

  %% Task format (required fields)
  TF["TASK FORMAT<br/>Required fields"]:::heading
  TF --> T1["type<br/>which handler runs"]
  TF --> T2["retries<br/>max retry attempts"]
  TF --> T3["payload<br/>free-form key/value data"]
  TF --> T4["attempts<br/>tracked internally by worker"]

  classDef heading fill:#111827,color:#ffffff,stroke:#374151,stroke-width:1px;
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


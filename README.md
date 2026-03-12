# logging

Structured logging package for Go services. Wraps [Uber Zap](https://github.com/uber-go/zap) with field constants, event names, error types, and automatic context injection.

## Install

```bash
go get github.com/onepercentclub-io/logging
```

## Quick Start

```go
package main

import (
    "context"

    "github.com/onepercentclub-io/logging"
    "github.com/onepercentclub-io/logging/events"
    "github.com/onepercentclub-io/logging/fields"
)

func main() {
    // Initialize once at startup
    logging.Init(logging.Config{
        Service:     "investments-backend",
        Environment: "prod",
        SentryDSN:   "https://examplePublicKey@o0.ingest.sentry.io/0",
    })

    // Startup logging (no request context)
    logging.Get().Infow(events.AppStarted, "version", "1.0.0")

    // Request-scoped logging (in handlers/middleware)
    ctx := context.Background()
    ctx = logging.WithUserID(ctx, "usr_789")
    ctx = logging.WithRequestID(ctx, "req_abc-123")

    log := logging.GetLogger(ctx)
    // user_id, request_id, trace_id are auto-injected — no need to pass them manually
    log.Infow(events.HTTPRequestCompleted,
        logging.HTTPFields("POST", "/api/v1/payments", 200, 234)...,
    )
}
```

**Production output** (JSON, one line per log):
```json
{"level":20,"time":"2026-03-12T12:30:50.000Z","msg":"http_request_completed","service":"investments-backend","environment":"prod","trace_id":"abc123","user_id":"usr_789","request_id":"req_abc-123","http.method":"POST","http.path":"/api/v1/payments","http.status_code":200,"duration_ms":234}
```

Every field is queryable in CloudWatch:
```
filter user_id = "usr_789" and http.status_code >= 500
```

## Package Structure

```
github.com/onepercentclub-io/logging
├── fields/    # Field name constants (e.g. fields.UserID = "user_id")
├── events/    # Event name constants (e.g. events.TaskFailed = "task_failed")
└── errors/    # Error type constants (e.g. errors.GatewayTimeout = "GATEWAY_TIMEOUT")
```

## API Reference

### Initialization

```go
// Call once at startup. Panics if Service is empty.
// Subsequent calls are no-ops.
logging.Init(logging.Config{
    Service:     "my-service",      // required
    Environment: "prod",            // "local" uses dev-friendly console output
    SentryDSN:   "https://...",     // optional, enables Sentry integration
})
```

### Getting a Logger

```go
// With request context — auto-injects trace_id, user_id, request_id
log := logging.GetLogger(ctx)

// Without context — for startup/shutdown logging
log := logging.Get()
```

`GetLogger` never mutates global state. Each call returns a fresh logger instance with context fields attached via Zap's `With()`.

### Logging Events

Always use `Infow`/`Errorw`/`Warnw` (the **w** variants) with event constants as the message:

```go
log.Infow(events.TaskCompleted,
    logging.TaskFields("task_001", "process_payment", "asynq")...,
)
```

Never use `Infof`/`Errorf` — data embedded in format strings is not queryable.

### Alertw

For critical failures that need immediate on-call attention:

```go
log.Alertw("sip_execution_failed",
    fields.ErrorType, "BSE_ERROR",
    fields.ErrorMessage, "execution plan not found",
)
```

`Alertw` logs at Error level and sets the Sentry `alert` tag on the current span for alert routing.

### Context Injection

Set values in middleware, they appear in every downstream log automatically:

```go
// In auth middleware
ctx = logging.WithUserID(ctx, payload.ID)

// In APM middleware
ctx = logging.WithRequestID(ctx, uuid.New().String())

// Store Sentry span for trace_id extraction
ctx = context.WithValue(ctx, logging.SentryTransactionKey, span)
```

Extract values back when needed:

```go
userID, ok := logging.UserIDFromContext(ctx)
reqID, ok := logging.RequestIDFromContext(ctx)
```

## Helper Functions

Helpers reduce boilerplate and enforce field consistency. Each returns `[]interface{}` for use with Zap's `Infow`/`Errorw`:

```go
// HTTP request fields
logging.HTTPFields(method, path string, statusCode int, durationMs int64)

// External API call fields
logging.APICallFields(domain, method string, statusCode int, durationMs int64)

// Error fields (error.message omitted if err is nil)
logging.ErrorFields(errorType string, err error, isRetryable bool)

// Database operation fields
logging.DBFields(collection, operation string, queryMs int64)

// Asynq task fields
logging.TaskFields(taskID, taskName, taskType string)

// Duration from start time
logging.WithDuration(start time.Time)

// Combine multiple field slices
logging.MergeFields(fieldSets ...[]interface{})
```

### Combining Helpers

```go
log.Errorw(events.APICallFailed,
    logging.MergeFields(
        logging.APICallFields("api.razorpay.com", "POST", 504, 5234),
        logging.ErrorFields(errors.GatewayTimeout, err, true),
    )...,
)
```

## Field Constants

All field names are constants — use them instead of raw strings to prevent typos.

| Category | Constants | Example |
|----------|-----------|---------|
| Core | `fields.Service`, `fields.Environment`, `fields.TraceID`, `fields.SpanID`, `fields.UserID`, `fields.RequestID` | Auto-injected |
| HTTP | `fields.HTTPMethod`, `fields.HTTPPath`, `fields.HTTPStatusCode`, `fields.HTTPDomain`, `fields.HTTPClientIP` | `"http.method"` |
| Error | `fields.ErrorType`, `fields.ErrorMessage`, `fields.ErrorIsRetryable` | `"error.type"` |
| Timing | `fields.DurationMs` | `"duration_ms"` |
| Database | `fields.DBCollection`, `fields.DBOperation`, `fields.DBQueryMs` | `"db.collection"` |
| Task | `fields.TaskID`, `fields.TaskName`, `fields.TaskType`, `fields.QueuePendingCount`, `fields.QueueActiveCount`, `fields.QueueName` | `"task.id"` |
| Provider | `fields.ProviderName`, `fields.ProviderRequestID` | `"provider.name"` |

## Event Constants

Use as the first argument to `Infow`/`Errorw`:

| Event | Value |
|-------|-------|
| `events.HTTPRequestCompleted` | `"http_request_completed"` |
| `events.APICallStarted` | `"api_call_started"` |
| `events.APICallCompleted` | `"api_call_completed"` |
| `events.APICallFailed` | `"api_call_failed"` |
| `events.DBQueryCompleted` | `"db_query_completed"` |
| `events.DBQueryFailed` | `"db_query_failed"` |
| `events.TaskStarted` | `"task_started"` |
| `events.TaskCompleted` | `"task_completed"` |
| `events.TaskFailed` | `"task_failed"` |
| `events.WebhookReceived` | `"webhook_received"` |
| `events.WebhookProcessed` | `"webhook_processed"` |
| `events.WebhookFailed` | `"webhook_failed"` |
| `events.AppStarted` | `"app_started"` |
| `events.AppShutdown` | `"app_shutdown"` |

## Error Type Constants

Use with `fields.ErrorType` for classifiable error alerting:

| Category | Constants |
|----------|-----------|
| External | `errors.GatewayTimeout`, `errors.ServiceDown`, `errors.RateLimit`, `errors.BadGateway`, `errors.ConnectionRefused` |
| Database | `errors.DBConnection`, `errors.DBQuery`, `errors.DBTimeout`, `errors.DBNotFound`, `errors.DBDuplicate` |
| Validation | `errors.Validation`, `errors.InvalidInput`, `errors.MissingField` |
| Auth | `errors.Unauthorized`, `errors.Forbidden`, `errors.TokenExpired` |
| Internal | `errors.Internal`, `errors.Panic` |

## Service-Specific Extensions

This package contains only **shared** constants. Each service defines its own domain-specific constants in its own repo:

```go
// investments-backend/internal/logging/fields.go
package logging

const (
    PaymentID    = "payment_id"
    OrderID      = "order_id"
    BasketID     = "basket_id"
    // ...
)
```

Import both the shared package and your service extensions:

```go
import (
    "github.com/onepercentclub-io/logging"
    "github.com/onepercentclub-io/logging/fields"
    "github.com/onepercentclub-io/logging/events"

    investlog "investments-backend/internal/logging"
)

log.Infow(investlog.PaymentCreated,
    investlog.PaymentID, payment.ID,
    fields.ProviderName, "razorpay",
)
```

## Rules

1. Use `Infow`/`Errorw`/`Warnw` — never `Infof`/`Errorf`
2. Use field constants — never raw strings for field names
3. Use event constants as the message — enables `filter msg = "payment_failed"`
4. Don't pass `user_id`/`trace_id`/`request_id` manually — they're auto-injected from context
5. Don't log in loops — log a summary after
6. Don't dump entire structs — log only the fields you need
7. Don't log PII (PAN, Aadhaar, bank accounts, tokens)

## Local Development

```bash
# Run tests
go test ./... -v

# Run tests with race detector
go test ./... -v -race

# Run example
go run ./example/
```

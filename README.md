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
    // Initialize once at startup. The package itself is vendor-neutral —
    // it only depends on Zap and the OpenTelemetry trace API. To ship errors
    // to Sentry, Datadog, etc., build that core in your service and inject
    // it via Config.ExtraCores (see "Wiring an error sink" below).
    logging.Init(logging.Config{
        Service:     "investments-backend",
        Environment: "prod",
    })

    // Startup logging (no request context)
    logging.Get().Infow(events.AppStarted, "version", "1.0.0")

    // Request-scoped logging (in handlers/middleware)
    ctx := context.Background()
    ctx = logging.WithUserID(ctx, "usr_789")
    ctx = logging.WithRequestID(ctx, "req_abc-123")

    log := logging.GetLogger(ctx)
    // user_id, request_id, trace_id, span_id are auto-injected — no need to
    // pass them manually. trace_id / span_id come from the active OTel span,
    // user_id / request_id come from middleware setters.
    log.Infow(events.HTTPRequestCompleted,
        logging.HTTPFields("POST", "/api/v1/payments", 200, 234)...,
    )
}
```

**Output** (JSON, one line per log — identical shape in every environment,
including local):
```json
{"level":30,"time":"2026-03-12T12:30:50.000Z","msg":"http_request_completed","service":"investments-backend","environment":"prod","trace_id":"0af7651916cd43dd8448eb211c80319c","span_id":"b7ad6b7169203331","user_id":"usr_789","request_id":"req_abc-123","http.method":"POST","http.path":"/api/v1/payments","http.status_code":200,"duration_ms":234}
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
    Environment: "prod",            // "local" enables debug level + disables sampling; output is always JSON
    Sampling:    logging.Sampling{Initial: 100, Thereafter: 100}, // optional; defaults applied in non-local
    ExtraCores:  []zapcore.Core{sentryCore}, // optional; see "Wiring an error sink"
})
```

### Trace correlation

`trace_id` and `span_id` are extracted from the active OpenTelemetry span on
the context — via `go.opentelemetry.io/otel/trace.SpanFromContext(ctx)`. This
works with any OTel-compatible tracer:

- OTel-native (OpenTelemetry SDK + an exporter)
- Sentry's OTel bridge
- Datadog's OTel integration
- Any other backend that participates in OTel context propagation

No setup is required inside this package — as long as your service starts
spans on incoming requests, `GetLogger(ctx)` finds them automatically. If no
tracer is wired, the fields are simply omitted from log lines (the no-op span
returns an invalid `SpanContext`).

### Wiring an error sink (Sentry, Datadog, ...)

This package no longer creates a Sentry client itself. Build whatever
`zapcore.Core` you need in your service and pass it in via `Config.ExtraCores`.
Every log entry is teed to stdout *and* to each extra core.

```go
import (
    "github.com/TheZeroSlave/zapsentry"
    "github.com/getsentry/sentry-go"
    "go.uber.org/zap/zapcore"

    "github.com/onepercentclub-io/logging"
)

// In your service's startup (you already have one Sentry client — reuse it):
sentryCore, _ := zapsentry.NewCore(zapsentry.Configuration{
    Level: zapcore.ErrorLevel,
}, zapsentry.NewSentryClientFromClient(sentry.CurrentHub().Client()))

logging.Init(logging.Config{
    Service:     "investments-backend",
    Environment: env,
    ExtraCores:  []zapcore.Core{sentryCore},
})
```

This keeps the logging package vendor-neutral and avoids the double-Sentry-
client problem services would otherwise hit (one from `sentry.Init` in APM
middleware, a second from the logging package).

### Sampling

In non-local environments, Zap's per-second sampler thins repeated entries of
the same `(level, message)` tuple to cap CloudWatch volume on hot paths
(health checks, tight loops). For each tuple per second, the first `Initial`
entries are kept, then 1 in every `Thereafter` after that.

- Default in non-local: `Sampling{Initial: 100, Thereafter: 100}`.
- `local` always disables sampling so developers see every log.
- Set `DisableSampling: true` to disable sampling explicitly in prod.

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

`Alertw` logs at Error level and sets the `alert=true` attribute on the active
OpenTelemetry span, so on-call tooling can route critical failures
specifically. If no tracer is wired, the attribute write is a safe no-op and
the Errorw still fires.

### Context Injection

Set values in middleware, they appear in every downstream log automatically:

```go
// In auth middleware
ctx = logging.WithUserID(ctx, payload.ID)

// In APM middleware
ctx = logging.WithRequestID(ctx, uuid.New().String())
// trace_id and span_id are picked up automatically from the OTel span
// that your tracing middleware already starts on `ctx` — nothing extra to do.
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

// Queue saturation fields (asynq middleware at task pickup)
logging.QueueFields(queueName string, pending, active int)

// Cache operation fields
logging.CacheFields(key string, hit bool, ttlSeconds int)

// Retry attempt fields
logging.RetryFields(attempt, maxCount int, delayMs int64)

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
| HTTP | `fields.HTTPMethod`, `fields.HTTPPath`, `fields.HTTPStatusCode`, `fields.HTTPDomain`, `fields.HTTPClientIP`, `fields.HTTPUserAgent`, `fields.HTTPRequestID` | `"http.method"` |
| Error | `fields.ErrorType`, `fields.ErrorMessage`, `fields.ErrorIsRetryable`, `fields.ErrorCode`, `fields.ErrorStackTrace` | `"error.type"` |
| Timing | `fields.DurationMs`, `fields.DBQueryMsTotal`, `fields.ExternalAPIMs` | `"duration_ms"` |
| Database | `fields.DBCollection` / `fields.DBTable`, `fields.DBOperation`, `fields.DBQueryMs`, `fields.DBRowsAffected` | `"db.collection"` |
| Task | `fields.TaskID`, `fields.TaskName`, `fields.TaskType`, `fields.TaskStatus`, `fields.QueueName`, `fields.QueuePendingCount`, `fields.QueueActiveCount`, `fields.QueueDepth` | `"task.id"` |
| Cache | `fields.CacheHit`, `fields.CacheKey`, `fields.CacheTTL` | `"cache.hit"` |
| Retry | `fields.RetryAttempt`, `fields.RetryMaxCount`, `fields.RetryDelayMs` | `"retry.attempt"` |
| Provider | `fields.ProviderName`, `fields.ProviderRequestID`, `fields.ProviderResponseID` | `"provider.name"` |

> `db.collection` and `db.table` are aliases for the same concept — pick whichever
> matches the store (Mongo "collection" vs SQL "table") and stay consistent inside
> each service.

## Event Constants

Use as the first argument to `Infow`/`Errorw`:

| Event | Value |
|-------|-------|
| `events.HTTPRequestStarted` | `"http_request_started"` |
| `events.HTTPRequestCompleted` | `"http_request_completed"` |
| `events.APICallStarted` | `"api_call_started"` |
| `events.APICallCompleted` | `"api_call_completed"` |
| `events.APICallFailed` | `"api_call_failed"` |
| `events.DBQueryStarted` | `"db_query_started"` |
| `events.DBQueryCompleted` | `"db_query_completed"` |
| `events.DBQueryFailed` | `"db_query_failed"` |
| `events.TaskStarted` | `"task_started"` |
| `events.TaskCompleted` | `"task_completed"` |
| `events.TaskFailed` | `"task_failed"` |
| `events.TaskRetrying` | `"task_retrying"` |
| `events.WebhookReceived` | `"webhook_received"` |
| `events.WebhookProcessed` | `"webhook_processed"` |
| `events.WebhookFailed` | `"webhook_failed"` |
| `events.AuthLoginSuccess` | `"auth_login_success"` |
| `events.AuthLoginFailed` | `"auth_login_failed"` |
| `events.AuthTokenRefresh` | `"auth_token_refresh"` |
| `events.AuthTokenExpired` | `"auth_token_expired"` |
| `events.CacheHit` | `"cache_hit"` |
| `events.CacheMiss` | `"cache_miss"` |
| `events.CacheSet` | `"cache_set"` |
| `events.HealthCheck` | `"health_check"` |
| `events.ConfigLoaded` | `"config_loaded"` |
| `events.AppStarted` | `"app_started"` |
| `events.AppShutdown` | `"app_shutdown"` |

## Error Type Constants

Use with `fields.ErrorType` for classifiable error alerting:

| Category | Constants |
|----------|-----------|
| External | `errors.GatewayTimeout`, `errors.ServiceDown`, `errors.RateLimit`, `errors.BadGateway`, `errors.ConnectionRefused`, `errors.SSLError` |
| Database | `errors.DBConnection`, `errors.DBQuery`, `errors.DBTimeout`, `errors.DBNotFound`, `errors.DBDuplicate`, `errors.DBTransaction` |
| Validation | `errors.Validation`, `errors.InvalidInput`, `errors.MissingField`, `errors.InvalidFormat` |
| Auth | `errors.Unauthorized`, `errors.Forbidden`, `errors.TokenExpired`, `errors.InvalidToken` |
| Internal | `errors.Internal`, `errors.Panic`, `errors.ConfigError`, `errors.Serialization` |
| Retry / Circuit | `errors.MaxRetriesExceeded`, `errors.CircuitOpen` |

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

Output is structured JSON in every environment — local included — so the log
shape you see on your machine is exactly what CloudWatch ingests, and metric
filters keyed on `msg`/`level`/`time` can never diverge from what a service
emits. Locally the minimum level is debug and sampling is off; that's the only
difference from production. For a human-friendly view, pipe through `jq`:

```bash
# Run example (local: debug level, no sampling — still JSON)
go run ./example/

# Production behavior (info level, sampling on)
ENV=prod go run ./example/

# Pretty-print locally
go run ./example/ | jq .
```

## Migrating from the Sentry-coupled version

Earlier revisions of this package depended on `sentry-go` and `zapsentry`
directly and created their own Sentry client inside `Init`. That caused two
problems in production: services ended up with **two** Sentry clients with
different configs (one from the service's APM middleware, one from this
package), and `trace_id` extraction was hardcoded to `*sentry.Span`, which
breaks the moment a service migrates to OpenTelemetry.

The current version is **vendor-neutral**. If you are upgrading from the
Sentry-coupled version, three things change for callers:

1. **`Config.SentryDSN` is gone.** Drop it from your `logging.Init(...)`
   call. The compile error you'll see is intentional — it makes sure you
   don't silently lose Sentry integration.

2. **Wire `zapsentry` yourself.** Build the core in your service (using the
   Sentry client your APM middleware already created) and pass it via
   `Config.ExtraCores`. See [Wiring an error sink](#wiring-an-error-sink-sentry-datadog-) above for the snippet.

3. **`SentryTransactionKey` is gone.** Stop writing `*sentry.Span` to context
   under that key. `GetLogger(ctx)` now reads `trace_id` / `span_id` from the
   active OpenTelemetry span via `trace.SpanFromContext(ctx)`. Any
   OTel-compatible tracer (including Sentry's OTel bridge) feeds this
   automatically — there's nothing to set up in this package.

The structured logging API itself — `GetLogger(ctx)`, `Infow`/`Errorw`/
`Alertw`, all field/event/error constants, every helper — is unchanged.

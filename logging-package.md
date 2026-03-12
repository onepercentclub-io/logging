# Logging Package — PRD & Technical Specification

**Status**: Draft
**Author**: Engineering Team
**Last Updated**: 2026-03-11

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Goals & Non-Goals](#2-goals--non-goals)
3. [Architecture Decision](#3-architecture-decision)
4. [Package Structure](#4-package-structure)
5. [Component Specifications](#5-component-specifications)
   - 5.1 Logger Core (`logger.go`)
   - 5.2 Field Constants (`fields/`)
   - 5.3 Event Names (`events/`)
   - 5.4 Error Types (`errors/`)
   - 5.5 Helper Functions (`helpers.go`)
   - 5.6 Context Extraction (`context.go`)
   - 5.7 Sampling (`sampling.go`)
6. [Service-Specific Extensions](#6-service-specific-extensions)
7. [Middleware Integration](#7-middleware-integration)
8. [Migration Guide](#8-migration-guide)
9. [CloudWatch Query Cookbook](#9-cloudwatch-query-cookbook)
10. [Adoption Guidelines](#10-adoption-guidelines)
11. [Testing Requirements](#11-testing-requirements)
12. [Appendix: Current State Audit](#12-appendix-current-state-audit)

---

## 1. Problem Statement

### What's Broken Today

Our current logging system (`common_utils.GetLogger`) wraps Uber's Zap but provides no structure enforcement. The result: **logs that are expensive to store and impossible to query reliably**.

#### Problem 1: Data Buried in Format Strings

```go
// ACTUAL CODE — src/internal/usecase/payments/payments.go:137
common_utils.GetLogger(&ctx).Infof(
    "Initiating payment %s for user %s (current status: %s)",
    txnId, txn.UserID, txn.Status,
)
```

CloudWatch sees:
```json
{"msg": "Initiating payment pay_abc123 for user usr_789 (current status: CREATED)"}
```

To find this log, you need regex: `filter msg like /for user usr_789/`. This is **slow, expensive, and breaks** when anyone changes the message wording. The `payment_id`, `user_id`, and `status` are invisible to queries.

**Found in 100+ locations** across usecase, repository, and external layers using `Infof`/`Errorf` with embedded values.

#### Problem 2: Same Field, Different Names

```go
// src/delivery/asynq/middlewares/middlewares.go
// Line 29 — Sentry tags:
scope.SetTags(map[string]string{"taskId": t.ResultWriter().TaskID()})  // camelCase

// Line 39 — Log fields (10 lines later, same file):
log := common_utils.GetLogger(&sentryCtx).With("taskID", t.ResultWriter().TaskID())  // different case
```

| Concept | Names found in codebase |
|---------|------------------------|
| User ID | `"userId"`, `"user_id"`, `"userID"`, `user=%s` in message |
| Payment ID | `"paymentId"`, `"payment_id"`, `"txnId"`, `%s` in message |
| Task ID | `"taskId"`, `"taskID"` |
| Basket ID | `"basketId"`, `"basket=%s"` in message |

CloudWatch query `filter userId = "usr_789"` finds **some** logs. `filter user_id = "usr_789"` finds **different** logs. Neither finds all.

#### Problem 3: No Request Correlation

A user reports: "My payment failed." You have their user ID.

**Today**: Search `filter msg like /usr_789/` → 500 logs for that user across the day → manually eyeball timestamps → 15-30 minutes to find the error.

**Why**: No `trace_id` or `request_id` in logs. No way to see all logs from a single request. The Sentry span has a trace ID, but it's never written to the log output.

#### Problem 4: Errors Are Not Classifiable

```go
// src/internal/usecase/payments/payments.go:856
common_utils.GetLogger(&ctx).Errorf("Unhandled payment method in polling: %s", txn.PaymentMethod)

// src/internal/repository/execution/mongo.go:101
common_utils.GetLogger(&ctx).With(err).Error("error finding execution plan in mongodb")
```

Both are errors. One is a business logic gap, the other is a DB failure. There's no `error.type` field to distinguish them. You cannot build a CloudWatch alarm for "alert when DB errors spike" because DB errors look identical to every other error.

#### Problem 5: Global Logger Mutation Bug

```go
// src/packages/common_utils/logger.go:89-103
func GetLogger(ctx *context.Context) *StandardLogger {
    if logger != nil {
        if ctx != nil {
            // BUG: Mutates the GLOBAL singleton logger with request-specific fields
            logger = &StandardLogger{SugaredLogger: logger.With(GetLogScopeFromContext(*ctx))}
```

Every call to `GetLogger(&ctx)` adds fields to the **global** logger. Under concurrent requests, fields from request A leak into request B's logs. This is a data race on a package-level variable.

#### Problem 6: Shared Package Needed

We have multiple backend services. Each one needs the same logging patterns, field names, and context extraction. Without a shared package, each service will independently define `"user_id"` vs `"userId"` vs `"userID"` — recreating the exact problem we're trying to solve.

### Impact Summary

| Problem | Business Impact |
|---------|----------------|
| Unqueryable fields | MTTR 15-30 min per incident (vs 2 min with structured logs) |
| No correlation IDs | Cannot trace a request across services |
| No error classification | Cannot build targeted alerts (DB vs external vs business logic) |
| Inconsistent field names | CloudWatch dashboards show partial data |
| Global logger mutation | Log field contamination across concurrent requests |
| No shared package | Every new service recreates the same problems |

---

## 2. Goals & Non-Goals

### Goals

1. **Every log field is queryable** — no data buried in format strings
2. **Field names are consistent** — one name per concept, enforced via constants
3. **Every log has correlation IDs** — `trace_id`, `user_id`, `request_id` auto-injected from context
4. **Errors are classifiable** — `error.type` field enables targeted alerting
5. **Shared across services** — single `pkg/logging` package used by all backends
6. **Backward compatible** — existing `common_utils.GetLogger` can be migrated incrementally, not all-at-once
7. **Fix the global mutation bug** — `GetLogger` returns a new instance per call, never mutates global state

### Non-Goals

1. **Replace Zap** — we extend it, not replace it
2. **Replace Sentry** — APM stays as-is, we extract trace IDs from existing Sentry spans
3. **Log aggregation infrastructure** — this is about the application-side package, not CloudWatch/Grafana setup
4. **Sampling at application level** — defer to infrastructure (CloudWatch agent config) unless cost becomes critical
5. **Structured logging for HTTP request/response bodies** — the existing `httpUtil` already logs these; we improve field names but don't change the logging granularity

---

## 3. Architecture Decision

### Extend Zap, Don't Replace

| Aspect | Build from Scratch | Extend Zap (our choice) |
|--------|-------------------|-------------------------|
| Time to build | 1-2 weeks | 2-3 days |
| Sentry integration | Rebuild | Keep existing `zapsentry` |
| Performance | Unproven | Battle-tested |
| Risk | High | Low |
| Team familiarity | None | Already using Zap |

### What We Build vs What Zap Provides

**We build:**
- Field constants (compile-time safe field names)
- Event name constants (consistent log event identification)
- Error type constants (classifiable errors)
- Helper functions (reduce boilerplate for common patterns)
- Context extraction (auto-inject `trace_id`, `user_id`, `request_id`)
- Logger wrapper that fixes the global mutation bug

**Zap provides:**
- JSON encoding, async writes, log levels
- SugaredLogger API (`Infow`, `Errorw`, `Warnw`)
- Sentry integration via `zapsentry`

---

## 4. Package Structure

### Shared Package (used by all services)

```
pkg/logging/
├── logger.go           # Init(), GetLogger(ctx), Logger wrapper
├── context.go          # Context keys, FieldsFromContext(), auto-injection
├── helpers.go          # HTTPFields(), ErrorFields(), DBFields(), TaskFields()
├── fields/
│   └── fields.go       # Generic field constants (~30 fields)
├── events/
│   └── events.go       # Generic event constants (~20 events)
└── errors/
    └── errors.go       # Generic error type constants (~15 types)
```

### Service-Specific Extensions (per service)

```
investments-backend/
└── internal/logging/
    ├── fields.go       # payment_id, mandate_id, basket_id, folio_number, etc.
    ├── events.go       # payment_created, order_placed, sip_executed, etc.
    └── errors.go       # RAZORPAY_ERROR, BSE_ERROR, CASHFREE_ERROR, etc.
```

### Why This Split

- **Shared package** contains only what all services need — HTTP, DB, error, task fields
- **Service extensions** contain domain-specific constants — payment IDs, order statuses, vendor errors
- Adding a new field to investments-backend doesn't require updating the shared package
- New services get consistent base logging from day one by importing `pkg/logging`

---

## 5. Component Specifications

### 5.1 Logger Core (`logger.go`)

The logger wraps Zap's `SugaredLogger` and fixes the global mutation bug by **never modifying the base logger**. Every `GetLogger(ctx)` call returns a new `*Logger` instance with context fields attached via Zap's `With()`.

```go
package logging

import (
    "context"
    "sync"

    "github.com/TheZeroSlave/zapsentry"
    "github.com/getsentry/sentry-go"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"

    "pkg/logging/fields"
)

// Logger wraps Zap's SugaredLogger. Each instance is request-scoped
// and pre-populated with context fields (trace_id, user_id, etc).
// Safe for concurrent use within a single request; do NOT share across requests.
type Logger struct {
    *zap.SugaredLogger
    ctx context.Context // retained for Alertw's Sentry tag setting
}

// Config holds initialization parameters.
type Config struct {
    // Service is the service name added to every log line.
    // Example: "investments-backend", "users-backend"
    Service string

    // Environment is the deployment environment.
    // Example: "local", "dev", "staging", "prod"
    Environment string

    // SentryDSN is the Sentry DSN for error tracking integration.
    // If empty, Sentry integration is skipped.
    SentryDSN string
}

var (
    baseLogger  *zap.SugaredLogger
    serviceName string
    initOnce    sync.Once
)

// integerLevelEncoder matches the existing encoding: (level + 3) * 10
// Debug=10, Info=20, Warn=30, Error=40, DPanic=50, Panic=60, Fatal=70
func integerLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
    enc.AppendInt8((int8(l) + 3) * 10)
}

// Init initializes the global base logger. Must be called once at application startup,
// before any GetLogger() calls. Panics if called with empty Service name.
//
// Example:
//
//     logging.Init(logging.Config{
//         Service:     "investments-backend",
//         Environment: config.APP_ENV,
//         SentryDSN:   config.SENTRY_DSN,
//     })
func Init(cfg Config) {
    if cfg.Service == "" {
        panic("logging: Config.Service must not be empty")
    }

    initOnce.Do(func() {
        serviceName = cfg.Service

        var zapCfg zap.Config
        if cfg.Environment != "local" {
            zapCfg = zap.NewProductionConfig()
            zapCfg.OutputPaths = []string{"stdout"}
            zapCfg.ErrorOutputPaths = []string{"stdout"}
            zapCfg.InitialFields = map[string]interface{}{
                fields.Service:     cfg.Service,
                fields.Environment: cfg.Environment,
            }
            zapCfg.EncoderConfig.EncodeLevel = integerLevelEncoder
            zapCfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
            zapCfg.EncoderConfig.TimeKey = "time"
            zapCfg.DisableCaller = true
            zapCfg.DisableStacktrace = true
        } else {
            zapCfg = zap.NewDevelopmentConfig()
            zapCfg.OutputPaths = []string{"stdout"}
            zapCfg.ErrorOutputPaths = []string{"stderr"}
            zapCfg.InitialFields = map[string]interface{}{
                fields.Service: cfg.Service,
            }
        }

        zapLogger, err := zapCfg.Build()
        if err != nil {
            panic("logging: failed to build zap logger: " + err.Error())
        }

        // Attach Sentry core for error-level logs (non-local only)
        if cfg.Environment != "local" && cfg.SentryDSN != "" {
            client := sentry.CurrentHub().Clone().Client()
            if client != nil {
                sentryCore, err := zapsentry.NewCore(zapsentry.Configuration{
                    Level:             zapcore.ErrorLevel,
                    EnableBreadcrumbs: false,
                    BreadcrumbLevel:   zapcore.InfoLevel,
                }, zapsentry.NewSentryClientFromClient(client))
                if err == nil {
                    zapLogger = zapsentry.AttachCoreToLogger(sentryCore, zapLogger)
                }
            }
        }

        baseLogger = zapLogger.Sugar()
    })
}

// GetLogger returns a new Logger pre-populated with context fields.
//
// Auto-injected fields (if present in context):
//   - trace_id  (from Sentry span)
//   - span_id   (from Sentry span)
//   - user_id   (set by auth middleware)
//   - request_id (set by APM middleware)
//
// This function NEVER mutates the global base logger. Each call returns
// a fresh instance with fields attached via Zap's With().
//
// Example:
//
//     log := logging.GetLogger(ctx)
//     log.Infow(events.PaymentCreated,
//         fields.PaymentID, payment.ID,
//         fields.PaymentAmount, payment.Amount,
//     )
//     // Output includes trace_id, user_id, request_id automatically
func GetLogger(ctx context.Context) *Logger {
    if baseLogger == nil {
        panic("logging: Init() must be called before GetLogger()")
    }

    sugar := baseLogger

    if ctx == nil {
        return &Logger{SugaredLogger: sugar}
    }

    // Auto-inject trace_id and span_id from Sentry span
    if spanRef := ctx.Value(SentryTransactionKey); spanRef != nil {
        if span, ok := spanRef.(*sentry.Span); ok {
            sugar = sugar.With(
                fields.TraceID, span.TraceID.String(),
                fields.SpanID, span.SpanID.String(),
            )
        }
    }

    // Auto-inject user_id (set by auth middleware)
    if userID, ok := ctx.Value(ctxKeyUserID).(string); ok && userID != "" {
        sugar = sugar.With(fields.UserID, userID)
    }

    // Auto-inject request_id (set by APM middleware)
    if reqID, ok := ctx.Value(ctxKeyRequestID).(string); ok && reqID != "" {
        sugar = sugar.With(fields.RequestID, reqID)
    }

    // Attach Sentry scope for error routing
    sugar = sugar.With(getLogScopeFromContext(ctx))

    return &Logger{SugaredLogger: sugar, ctx: ctx}
}

// Get returns a logger without context (for startup/shutdown logging).
// Prefer GetLogger(ctx) in request/task handlers.
func Get() *Logger {
    if baseLogger == nil {
        panic("logging: Init() must be called before Get()")
    }
    return &Logger{SugaredLogger: baseLogger}
}

// Alertw logs at Error level AND sets the Sentry "alert" tag on the current span.
// Use this for critical business failures that require immediate on-call attention.
//
// This replaces the old GetLoggerWithAlerting() pattern. Instead of creating
// a separate logger instance, the alert is tied to a specific log event.
//
// When to use Alertw vs Errorw:
//   - Errorw: something failed, needs investigation, but the system can recover
//   - Alertw: something failed that needs IMMEDIATE human attention (on-call page)
//
// Current production usages (3 critical paths):
//   - KYC provider BAV credits exhausted (cannot verify bank accounts)
//   - KYC token expired during verification (user stuck in KYC flow)
//   - SIP batch execution plan retrieval failed (SIPs not executing)
//
// Example:
//
//     log := logging.GetLogger(ctx)
//     log.Alertw(investlog.SIPExecutionFailed,
//         investlog.SIPConfigID, configID,
//         fields.ErrorType, investlog.ErrBSE,
//         fields.ErrorMessage, err.Error(),
//     )
func (l *Logger) Alertw(msg string, keysAndValues ...interface{}) {
    // Set Sentry alert tag so Sentry alert rules can route to on-call
    if l.ctx != nil {
        if spanRef := l.ctx.Value(SentryTransactionKey); spanRef != nil {
            if span, ok := spanRef.(*sentry.Span); ok {
                span.SetTag("alert", "true")
            }
        }
    }
    l.SugaredLogger.Errorw(msg, keysAndValues...)
}
```

**Key design decisions:**

1. **`GetLogger` takes `context.Context` (not `*context.Context`)** — the current API takes a pointer to context which is non-idiomatic Go. The new API uses the standard `context.Context` interface. The migration section covers backward compatibility.

2. **Never mutates `baseLogger`** — every `GetLogger` call chains `.With()` which returns a new `SugaredLogger`. The global `baseLogger` is written once during `Init()` and never modified.

3. **Panics on misconfiguration** — `Init()` without a service name and `GetLogger()` before `Init()` are programmer errors that should fail loudly, not silently return nil.

4. **`Alertw` replaces `GetLoggerWithAlerting`** — the current codebase has `GetLoggerWithAlerting()` (used in 3 critical paths: KYC BAV credits, KYC token expiry, SIP batch failures). The old pattern created a separate logger instance that tagged ALL subsequent logs with `alert=true`. The new `Alertw` method ties the alert to a **specific log event**, which is more precise — you alert on the exact failure, not on everything the logger touches afterward. The Sentry `alert` tag is preserved for backward compatibility with existing alert rules.

---

### 5.2 Field Constants (`fields/fields.go`)

Constants prevent typos and enable IDE autocomplete. Organized by domain with flat dot-notation for nested concepts.

**Design rule**: Only add constants for fields that are **actually used**. Do not pre-define speculative fields.

```go
package fields

// ──────────────────────────────────────────────
// Core Fields (auto-injected by GetLogger — developers rarely type these)
// ──────────────────────────────────────────────

const (
    Service     = "service"
    Environment = "environment"
    TraceID     = "trace_id"
    SpanID      = "span_id"
    UserID      = "user_id"
    RequestID   = "request_id"
)

// ──────────────────────────────────────────────
// HTTP Fields
// Used in: REST middleware, external API call helpers
// ──────────────────────────────────────────────

const (
    HTTPMethod     = "http.method"
    HTTPPath       = "http.path"
    HTTPStatusCode = "http.status_code"
    HTTPDomain     = "http.domain"
    HTTPClientIP   = "http.client_ip"
)

// ──────────────────────────────────────────────
// Error Fields
// Used in: every layer that logs errors
// ──────────────────────────────────────────────

const (
    ErrorType       = "error.type"
    ErrorMessage    = "error.message"
    ErrorIsRetryable = "error.is_retryable"
)

// ──────────────────────────────────────────────
// Timing Fields
// Used in: middleware, external API calls, DB queries
// ──────────────────────────────────────────────

const (
    DurationMs = "duration_ms"
)

// ──────────────────────────────────────────────
// Database Fields
// Used in: repository layer logging
// ──────────────────────────────────────────────

const (
    DBCollection = "db.collection"
    DBOperation  = "db.operation"
    DBQueryMs    = "db.query_ms"
)

// ──────────────────────────────────────────────
// Task/Queue Fields
// Used in: asynq middleware, task handlers
// ──────────────────────────────────────────────

const (
    TaskID   = "task.id"
    TaskName = "task.name"
    TaskType = "task.type"

    // Queue saturation fields — logged by asynq middleware at task start.
    // These provide visibility into queue backlog (saturation signal).
    // Values come from asynq.Inspector.GetQueueInfo() at task pickup time.
    QueuePendingCount = "queue.pending_count" // tasks waiting to be processed
    QueueActiveCount  = "queue.active_count"  // tasks currently being processed
    QueueName         = "queue.name"          // queue name (e.g., "default", "critical")
)

// ──────────────────────────────────────────────
// External Provider Fields
// Used in: external/ layer when calling third-party APIs
// ──────────────────────────────────────────────

const (
    ProviderName      = "provider.name"
    ProviderRequestID = "provider.request_id"
)
```

**Why flat dot-notation** (`http.method` not `http_method`):
- CloudWatch Insights supports dot-notation natively: `filter http.status_code >= 500`
- Groups related fields visually in log output
- Matches OpenTelemetry semantic conventions

---

### 5.3 Event Names (`events/events.go`)

Every log message (`msg` field) should be a constant from this package. This makes CloudWatch queries deterministic — `filter msg = "api_call_failed"` always works.

```go
package events

// ──────────────────────────────────────────────
// HTTP Lifecycle
// ──────────────────────────────────────────────

const (
    // HTTPRequestCompleted is logged by the REST middleware after a request finishes.
    // Fields: http.method, http.path, http.status_code, duration_ms
    HTTPRequestCompleted = "http_request_completed"
)

// ──────────────────────────────────────────────
// External API Calls
// ──────────────────────────────────────────────

const (
    // APICallStarted is logged before making an external HTTP call.
    // Fields: http.domain, http.method, provider.name
    APICallStarted = "api_call_started"

    // APICallCompleted is logged after a successful external HTTP call.
    // Fields: http.domain, http.method, http.status_code, duration_ms, provider.name
    APICallCompleted = "api_call_completed"

    // APICallFailed is logged after a failed external HTTP call.
    // Fields: http.domain, http.method, http.status_code, duration_ms,
    //         error.type, error.message, error.is_retryable, provider.name
    APICallFailed = "api_call_failed"
)

// ──────────────────────────────────────────────
// Database Operations
// ──────────────────────────────────────────────

const (
    // DBQueryCompleted is logged after a DB operation completes.
    // Fields: db.collection, db.operation, db.query_ms
    DBQueryCompleted = "db_query_completed"

    // DBQueryFailed is logged after a DB operation fails.
    // Fields: db.collection, db.operation, db.query_ms, error.type, error.message
    DBQueryFailed = "db_query_failed"
)

// ──────────────────────────────────────────────
// Task/Queue Processing
// ──────────────────────────────────────────────

const (
    // TaskStarted is logged when an asynq task begins processing.
    // Fields: task.id, task.name, task.type
    TaskStarted = "task_started"

    // TaskCompleted is logged when an asynq task finishes successfully.
    // Fields: task.id, task.name, task.type, duration_ms
    TaskCompleted = "task_completed"

    // TaskFailed is logged when an asynq task fails.
    // Fields: task.id, task.name, task.type, duration_ms, error.type, error.message
    TaskFailed = "task_failed"
)

// ──────────────────────────────────────────────
// Webhook Processing
// ──────────────────────────────────────────────

const (
    // WebhookReceived is logged when an incoming webhook is received.
    // Fields: http.path, provider.name
    WebhookReceived = "webhook_received"

    // WebhookProcessed is logged after a webhook is successfully processed.
    // Fields: http.path, provider.name, duration_ms
    WebhookProcessed = "webhook_processed"

    // WebhookFailed is logged after a webhook processing fails.
    // Fields: http.path, provider.name, duration_ms, error.type, error.message
    WebhookFailed = "webhook_failed"
)

// ──────────────────────────────────────────────
// Application Lifecycle
// ──────────────────────────────────────────────

const (
    AppStarted  = "app_started"
    AppShutdown = "app_shutdown"
)
```

**Convention**: Every event constant has a doc comment listing the **expected fields**. This serves as both documentation and a contract — when a developer logs `events.APICallFailed`, the doc comment tells them exactly which fields to include.

---

### 5.4 Error Types (`errors/errors.go`)

Standard error classification enables targeted alerting. An error type describes **what category of failure** occurred, not the specific error message.

```go
package errors

// ──────────────────────────────────────────────
// External Service Errors
// Used when a third-party API call fails
// ──────────────────────────────────────────────

const (
    GatewayTimeout    = "GATEWAY_TIMEOUT"     // 504, context deadline exceeded
    ServiceDown       = "SERVICE_DOWN"         // 503, 502
    RateLimit         = "RATE_LIMIT"           // 429
    BadGateway        = "BAD_GATEWAY"          // 502
    ConnectionRefused = "CONNECTION_REFUSED"   // dial tcp: connection refused
)

// ──────────────────────────────────────────────
// Database Errors
// Used when a MongoDB/Redis operation fails
// ──────────────────────────────────────────────

const (
    DBConnection  = "DB_CONNECTION_ERROR"
    DBQuery       = "DB_QUERY_ERROR"
    DBTimeout     = "DB_TIMEOUT"
    DBNotFound    = "DB_NOT_FOUND"
    DBDuplicate   = "DB_DUPLICATE_KEY"
)

// ──────────────────────────────────────────────
// Validation Errors
// Used when input validation fails
// ──────────────────────────────────────────────

const (
    Validation   = "VALIDATION_ERROR"
    InvalidInput = "INVALID_INPUT"
    MissingField = "MISSING_FIELD"
)

// ──────────────────────────────────────────────
// Auth Errors
// ──────────────────────────────────────────────

const (
    Unauthorized = "UNAUTHORIZED"
    Forbidden    = "FORBIDDEN"
    TokenExpired = "TOKEN_EXPIRED"
)

// ──────────────────────────────────────────────
// Internal Errors
// ──────────────────────────────────────────────

const (
    Internal = "INTERNAL_ERROR"
    Panic    = "PANIC"
)
```

**Service-specific error types** live in the service's `internal/logging/errors.go` (see Section 6).

---

### 5.5 Helper Functions (`helpers.go`)

Helpers reduce boilerplate and enforce field consistency. Each helper returns `[]interface{}` compatible with Zap's `SugaredLogger.Infow()`.

```go
package logging

import (
    "time"

    "pkg/logging/fields"
)

// HTTPFields returns structured fields for an HTTP request log.
//
// Usage:
//
//     log.Infow(events.HTTPRequestCompleted,
//         logging.HTTPFields("POST", "/api/v1/payments", 200, 234)...,
//     )
func HTTPFields(method, path string, statusCode int, durationMs int64) []interface{} {
    return []interface{}{
        fields.HTTPMethod, method,
        fields.HTTPPath, path,
        fields.HTTPStatusCode, statusCode,
        fields.DurationMs, durationMs,
    }
}

// APICallFields returns structured fields for an external API call log.
//
// Usage:
//
//     log.Infow(events.APICallCompleted,
//         logging.APICallFields("api.razorpay.com", "POST", 200, 1234)...,
//     )
func APICallFields(domain, method string, statusCode int, durationMs int64) []interface{} {
    return []interface{}{
        fields.HTTPDomain, domain,
        fields.HTTPMethod, method,
        fields.HTTPStatusCode, statusCode,
        fields.DurationMs, durationMs,
    }
}

// ErrorFields returns structured fields for error logging.
//
// Usage:
//
//     log.Errorw(events.APICallFailed,
//         logging.ErrorFields(errors.GatewayTimeout, err, true)...,
//     )
func ErrorFields(errorType string, err error, isRetryable bool) []interface{} {
    f := []interface{}{
        fields.ErrorType, errorType,
        fields.ErrorIsRetryable, isRetryable,
    }
    if err != nil {
        f = append(f, fields.ErrorMessage, err.Error())
    }
    return f
}

// DBFields returns structured fields for a database operation log.
//
// Usage:
//
//     log.Infow(events.DBQueryCompleted,
//         logging.DBFields("payments", "find_one", 45)...,
//     )
func DBFields(collection, operation string, queryMs int64) []interface{} {
    return []interface{}{
        fields.DBCollection, collection,
        fields.DBOperation, operation,
        fields.DBQueryMs, queryMs,
    }
}

// TaskFields returns structured fields for an asynq task log.
//
// Usage:
//
//     log.Infow(events.TaskStarted,
//         logging.TaskFields(taskID, taskName, "asynq")...,
//     )
func TaskFields(taskID, taskName, taskType string) []interface{} {
    return []interface{}{
        fields.TaskID, taskID,
        fields.TaskName, taskName,
        fields.TaskType, taskType,
    }
}

// WithDuration returns a duration_ms field computed from a start time.
//
// Usage:
//
//     start := time.Now()
//     // ... operation ...
//     log.Infow("operation_completed",
//         logging.WithDuration(start)...,
//     )
func WithDuration(start time.Time) []interface{} {
    return []interface{}{
        fields.DurationMs, time.Since(start).Milliseconds(),
    }
}

// MergeFields concatenates multiple field slices into one.
//
// Usage:
//
//     log.Errorw(events.APICallFailed,
//         logging.MergeFields(
//             logging.APICallFields("api.razorpay.com", "POST", 504, 5234),
//             logging.ErrorFields(errors.GatewayTimeout, err, true),
//         )...,
//     )
func MergeFields(fieldSets ...[]interface{}) []interface{} {
    var total int
    for _, f := range fieldSets {
        total += len(f)
    }
    merged := make([]interface{}, 0, total)
    for _, f := range fieldSets {
        merged = append(merged, f...)
    }
    return merged
}
```

---

### 5.6 Context Extraction (`context.go`)

Defines context keys and extraction logic used by `GetLogger()` to auto-inject fields.

```go
package logging

import (
    "context"

    "github.com/TheZeroSlave/zapsentry"
    "github.com/getsentry/sentry-go"
    "go.uber.org/zap/zapcore"
)

// Context keys for auto-injected fields.
// These are set by middleware and extracted by GetLogger().
type contextKey string

const (
    // ctxKeyUserID stores the authenticated user's ID in context.
    // Set by: Auth middleware (after JWT verification)
    // Used by: GetLogger() to auto-inject user_id
    ctxKeyUserID contextKey = "logging_user_id"

    // ctxKeyRequestID stores a unique request identifier in context.
    // Set by: APM middleware (generated UUID per request)
    // Used by: GetLogger() to auto-inject request_id
    ctxKeyRequestID contextKey = "logging_request_id"
)

// SentryTransactionKey — BACKWARD COMPATIBILITY
//
// CRITICAL: This key must match the EXACT type and value used by the existing
// common_utils.SENTRY_TRANSACTION_KEY during the migration period.
//
// The current codebase defines this as:
//     type ContextKey string  // in common_utils/apm.go
//     const SENTRY_TRANSACTION_KEY ContextKey = "sentry_transaction"
//
// Go's context.Value() matches on BOTH type and value. If we define a new type
// (logging.contextKey) with the same string value ("sentry_transaction"), the
// lookup will FAIL silently — context.Value() returns nil because the types differ.
//
// This affects 7 files that read/write this key:
//   - delivery/rest/middlewares/apm.go (writes span on HTTP request)
//   - delivery/asynq/middlewares/middlewares.go (writes span on task)
//   - packages/common_utils/apm.go (WithSpan, PopulateSentryTraceHeaders)
//   - packages/common_utils/logger.go (reads span for tags)
//   - usecase/mf/transactions.go (propagates to goroutines)
//   - usecase/payments/helper.go (propagates to goroutines)
//   - usecase/execution/mf/execution_plan.go (propagates to goroutines)
//
// APPROACH: During Phases 1-3, import the existing key directly.
// In Phase 4 (cleanup), when common_utils is removed, move the type here.
//
// Phase 1-3 implementation:
import "github.com/one/backend/investments/src/packages/common_utils"

var SentryTransactionKey = common_utils.SENTRY_TRANSACTION_KEY

// Phase 4 implementation (after common_utils removal):
// type contextKey string  // already defined above
// const SentryTransactionKey contextKey = "sentry_transaction"

// WithUserID returns a new context with the user ID set.
// Call this in auth middleware after verifying the JWT.
//
// Example:
//
//     ctx = logging.WithUserID(ctx, payload.ID)
func WithUserID(ctx context.Context, userID string) context.Context {
    return context.WithValue(ctx, ctxKeyUserID, userID)
}

// WithRequestID returns a new context with the request ID set.
// Call this in APM middleware at the start of each request.
//
// Example:
//
//     ctx = logging.WithRequestID(ctx, uuid.New().String())
func WithRequestID(ctx context.Context, requestID string) context.Context {
    return context.WithValue(ctx, ctxKeyRequestID, requestID)
}

// UserIDFromContext extracts the user ID from context, if present.
func UserIDFromContext(ctx context.Context) (string, bool) {
    userID, ok := ctx.Value(ctxKeyUserID).(string)
    return userID, ok && userID != ""
}

// RequestIDFromContext extracts the request ID from context, if present.
func RequestIDFromContext(ctx context.Context) (string, bool) {
    reqID, ok := ctx.Value(ctxKeyRequestID).(string)
    return reqID, ok && reqID != ""
}

// getLogScopeFromContext returns the Sentry scope as a Zap field.
// Used internally by GetLogger().
func getLogScopeFromContext(ctx context.Context) zapcore.Field {
    hub := sentry.GetHubFromContext(ctx)
    if hub != nil {
        return zapsentry.NewScopeFromScope(hub.Scope().Clone())
    }
    return zapsentry.NewScope()
}
```

---

### 5.7 Sampling (`sampling.go`)

**Deferred**. Not included in v1. Reasons:

1. Current log volume doesn't justify application-level sampling
2. Sampling is better handled at the infrastructure layer (CloudWatch agent config, log router)
3. Premature sampling risks hiding errors in low-volume paths

If CloudWatch costs become a concern, add sampling as a v2 feature with these rules:
- Errors: always 100%
- Health checks: 1%
- Successful requests: 10-50% (configurable per service)

---

## 6. Service-Specific Extensions

Each service defines its own domain-specific constants in `internal/logging/`. These extend the shared package without modifying it.

### investments-backend — `internal/logging/fields.go`

```go
package logging

// ──────────────────────────────────────────────
// Investment Domain Fields
// ──────────────────────────────────────────────

const (
    PaymentID     = "payment_id"
    OrderID       = "order_id"
    TransactionID = "transaction_id"
    ExecutionID   = "execution_id"
    SIPConfigID   = "sip_config_id"
    MandateID     = "mandate_id"
    FolioNumber   = "folio_number"
    SchemeCode    = "scheme_code"
    AMCCode       = "amc_code"
    BasketID      = "basket_id"

    PaymentAmount  = "payment.amount"
    PaymentStatus  = "payment.status"
    PaymentMethod  = "payment.method"
    OrderStatus    = "order.status"
    MandateStatus  = "mandate.status"
)
```

### investments-backend — `internal/logging/events.go`

```go
package logging

// ──────────────────────────────────────────────
// Payment Events
// ──────────────────────────────────────────────

const (
    // PaymentCreated — a new payment record was created.
    // Fields: payment_id, payment.amount, payment.method, user_id
    PaymentCreated = "payment_created"

    // PaymentInitiated — payment sent to gateway.
    // Fields: payment_id, payment.method, provider.name
    PaymentInitiated = "payment_initiated"

    // PaymentCompleted — payment confirmed successful.
    // Fields: payment_id, payment.amount, payment.method, duration_ms
    PaymentCompleted = "payment_completed"

    // PaymentFailed — payment failed or timed out.
    // Fields: payment_id, payment.method, error.type, error.message
    PaymentFailed = "payment_failed"

    // PaymentStatusAnomaly — unexpected status transition (e.g., FAILED → SUCCESS).
    // Fields: payment_id, payment.status (current), error.message (description)
    PaymentStatusAnomaly = "payment_status_anomaly"
)

// ──────────────────────────────────────────────
// Order Events
// ──────────────────────────────────────────────

const (
    OrderCreated   = "order_created"
    OrderPlaced    = "order_placed"
    OrderCompleted = "order_completed"
    OrderFailed    = "order_failed"
    OrderCancelled = "order_cancelled"
)

// ──────────────────────────────────────────────
// Mandate Events
// ──────────────────────────────────────────────

const (
    MandateCreated   = "mandate_created"
    MandateActivated = "mandate_activated"
    MandateFailed    = "mandate_failed"
)

// ──────────────────────────────────────────────
// SIP Events
// ──────────────────────────────────────────────

const (
    SIPCreated  = "sip_created"
    SIPExecuted = "sip_executed"
    SIPPaused   = "sip_paused"
    SIPSkipped  = "sip_skipped"
)

// ──────────────────────────────────────────────
// Execution Events
// ──────────────────────────────────────────────

const (
    ExecutionPlanCreated   = "execution_plan_created"
    ExecutionPlanConfirmed = "execution_plan_confirmed"
    ExecutionPlanCompleted = "execution_plan_completed"
    ExecutionPlanFailed    = "execution_plan_failed"
)

// ──────────────────────────────────────────────
// Basket Events
// ──────────────────────────────────────────────

const (
    BasketRebalanceStarted   = "basket_rebalance_started"
    BasketRebalanceCompleted = "basket_rebalance_completed"
    BasketRedemptionStarted  = "basket_redemption_started"
)
```

### investments-backend — `internal/logging/errors.go`

```go
package logging

// ──────────────────────────────────────────────
// Provider-Specific Error Types
// ──────────────────────────────────────────────

const (
    ErrRazorpay  = "RAZORPAY_ERROR"
    ErrCashfree  = "CASHFREE_ERROR"
    ErrBSE       = "BSE_ERROR"
    ErrSmallcase = "SMALLCASE_ERROR"
    ErrDigio     = "DIGIO_ERROR"
)

// ──────────────────────────────────────────────
// Business Logic Error Types
// ──────────────────────────────────────────────

const (
    ErrInsufficientFunds   = "INSUFFICIENT_FUNDS"
    ErrMandateInactive     = "MANDATE_INACTIVE"
    ErrOrderNotFound       = "ORDER_NOT_FOUND"
    ErrDuplicateRequest    = "DUPLICATE_REQUEST"
    ErrOperationNotAllowed = "OPERATION_NOT_ALLOWED"
    ErrKYCIncomplete       = "KYC_INCOMPLETE"
)
```

### Usage — Combining Shared + Service-Specific

```go
import (
    "pkg/logging"
    "pkg/logging/fields"
    "pkg/logging/events"

    investlog "investments-backend/internal/logging"
)

func (uu *paymentsUsecase) InitiatePayment(ctx context.Context, txnId string) error {
    log := logging.GetLogger(ctx)
    // trace_id, user_id, request_id are already in `log` automatically

    log.Infow(investlog.PaymentInitiated,
        investlog.PaymentID, txnId,                        // service-specific
        investlog.PaymentMethod, txn.PaymentMethod,        // service-specific
        fields.ProviderName, "razorpay",                   // shared
    )

    res, err := paymentSvc.CreatePayment(ctx, payload)
    if err != nil {
        log.Errorw(investlog.PaymentFailed,
            investlog.PaymentID, txnId,
            fields.ErrorType, investlog.ErrRazorpay,       // service-specific error type
            fields.ErrorMessage, err.Error(),              // shared error field
            fields.ErrorIsRetryable, true,                 // shared error field
        )
        return domain.CustomError(err)
    }
    return nil
}
```

---

## 7. Middleware Integration

### 7.1 REST APM Middleware Changes

Current file: `src/delivery/rest/middlewares/apm.go`

**Changes needed:**

1. Generate `request_id` and store in context
2. Store `user_id` in context (after auth middleware runs, or in auth middleware itself)

```go
// In NewAPMMiddleware — after creating the Sentry span:

span := sentry.StartSpan(sentryCtx, "Incoming Request",
    sentry.WithTransactionName(c.Path()),
    sentry.ContinueFromRequest(r),
)

// NEW: Generate and store request_id
requestID := uuid.New().String()
sentryCtx = context.WithValue(sentryCtx, logging.ctxKeyRequestID, requestID)

// Existing: store Sentry span
sentryCtx = context.WithValue(sentryCtx, logging.SentryTransactionKey, span)
c.SetUserContext(sentryCtx)
```

**For HTTP request completion logging** — add at the end of the middleware:

```go
// After c.Next() returns:
log := logging.GetLogger(c.UserContext())
log.Infow(events.HTTPRequestCompleted,
    logging.HTTPFields(c.Method(), c.Path(), c.Response().StatusCode(),
        time.Since(start).Milliseconds())...,
)
```

### 7.2 Auth Middleware Changes

Current file: `src/delivery/rest/middlewares/auth.go`

**Change**: After verifying JWT and storing in `c.Locals("user")`, also store user ID in context for logging:

```go
// Existing (keep):
c.Locals("user", payload)

// NEW: Store user_id in context for logging auto-injection
ctx := c.UserContext()
ctx = logging.WithUserID(ctx, payload.ID)
c.SetUserContext(ctx)
```

This is the **only** middleware change needed. All downstream `logging.GetLogger(ctx)` calls will now automatically include `user_id`.

### 7.3 Asynq Task Middleware Changes

Current file: `src/delivery/asynq/middlewares/middlewares.go`

**Replace the current `Logging` middleware:**

```go
type middlewares struct {
    config    *utils.Config
    inspector *asynq.Inspector // NEW: for queue saturation metrics
}

func NewMiddlewares(config *utils.Config, logger *common_utils.StandardLogger, redisAddr string) Middlewares {
    return &middlewares{
        config:    config,
        inspector: asynq.NewInspector(asynq.RedisClientOpt{Addr: redisAddr}),
    }
}

func (m *middlewares) Logging(h asynq.Handler) asynq.Handler {
    return asynq.HandlerFunc(func(ctx context.Context, t *asynq.Task) error {
        hub := sentry.CurrentHub().Clone()
        scope := hub.Scope()

        taskID := t.ResultWriter().TaskID()
        taskName := t.Type()

        scope.SetTags(map[string]string{
            "type":      "task",
            "task.id":   taskID,        // consistent with field constant
            "task.name": taskName,      // consistent with field constant
        })

        sentryCtx := sentry.SetHubOnContext(ctx, hub)
        span := sentry.StartSpan(sentryCtx, "Task Execution",
            sentry.WithTransactionName(taskName))
        sentryCtx = context.WithValue(sentryCtx, logging.SentryTransactionKey, span)
        defer span.Finish()

        log := logging.GetLogger(sentryCtx)
        start := time.Now()

        // Build task start fields with queue saturation data
        startFields := logging.TaskFields(taskID, taskName, "asynq")
        if queueInfo, err := m.inspector.GetQueueInfo("default"); err == nil {
            startFields = logging.MergeFields(startFields, []interface{}{
                fields.QueueName, "default",
                fields.QueuePendingCount, queueInfo.Pending,
                fields.QueueActiveCount, queueInfo.Active,
            })
        }

        log.Infow(events.TaskStarted, startFields...)

        err := h.ProcessTask(sentryCtx, t)

        if err != nil {
            log.Errorw(events.TaskFailed,
                logging.MergeFields(
                    logging.TaskFields(taskID, taskName, "asynq"),
                    logging.ErrorFields(logerrors.Internal, err, true),
                    logging.WithDuration(start),
                )...,
            )
            span.Status = sentry.SpanStatusUnknown
            return domain.CustomError(err)
        }

        log.Infow(events.TaskCompleted,
            logging.MergeFields(
                logging.TaskFields(taskID, taskName, "asynq"),
                logging.WithDuration(start),
            )...,
        )
        span.Status = sentry.SpanStatusOK
        return nil
    })
}
```

**Queue saturation data**: The `inspector.GetQueueInfo()` call is cheap (single Redis command) and provides `Pending` and `Active` counts. Logging these at task start gives visibility into queue backlog — if `queue.pending_count` is climbing while `queue.active_count` is at max, the workers are saturated.

### 7.4 HTTP Client Wrapper Changes

Current file: `src/packages/common_utils/http.go`

The existing `httpUtil` already logs requests and responses. The change is to use **structured fields** instead of JSON-marshaling everything into the message:

```go
// BEFORE (current — http.go Post method):
log, _ := json.Marshal(map[string]interface{}{
    "headers": req.Header, "body": string(payload), "url": url, "method": "POST",
})
GetLogger(&ctx).Info(string(log))

// AFTER:
logging.GetLogger(ctx).Infow(events.APICallStarted,
    fields.HTTPDomain, req.URL.Hostname(),
    fields.HTTPMethod, "POST",
    fields.HTTPPath, req.URL.Path,
)

// ... execute request ...

// BEFORE (current):
GetLogger(&ctx).With("duration_ms", duration.Milliseconds()).Info(bodystr)

// AFTER (success):
logging.GetLogger(ctx).Infow(events.APICallCompleted,
    logging.APICallFields(req.URL.Hostname(), "POST", resp.StatusCode,
        duration.Milliseconds())...,
)

// AFTER (failure):
logging.GetLogger(ctx).Errorw(events.APICallFailed,
    logging.MergeFields(
        logging.APICallFields(req.URL.Hostname(), "POST", resp.StatusCode,
            duration.Milliseconds()),
        logging.ErrorFields(classifyHTTPError(resp.StatusCode), err, isRetryable(resp.StatusCode)),
    )...,
)
```

**Add a helper** to classify HTTP error codes:

```go
// In helpers.go or in the httpUtil package

func classifyHTTPError(statusCode int) string {
    switch {
    case statusCode == 429:
        return logerrors.RateLimit
    case statusCode == 502:
        return logerrors.BadGateway
    case statusCode == 503:
        return logerrors.ServiceDown
    case statusCode == 504:
        return logerrors.GatewayTimeout
    case statusCode >= 500:
        return logerrors.Internal
    default:
        return logerrors.Internal
    }
}

func isRetryable(statusCode int) bool {
    return statusCode == 429 || statusCode == 502 || statusCode == 503 || statusCode == 504
}
```

#### Preserving `NO_LOG` and `NO_LOG_REQUEST_BODY` Flags

The current `httpUtil` has two flags that suppress logging for specific HTTP calls. These **must be preserved** during migration:

| Flag | Usage Count | Purpose | Used By |
|------|-------------|---------|---------|
| `NO_LOG` | 11 calls | Suppresses all request/response logging | Market data polling (8), OneBackend internal calls (2), other (1) |
| `NO_LOG_REQUEST_BODY` | 1 call | Suppresses request body only (keeps headers) | BSE SOAP XML uploads |

**Why these exist**: Without `NO_LOG`, every market data poll (runs every few seconds) generates a log line with full request/response body. This would dominate log volume and CloudWatch costs. Without `NO_LOG_REQUEST_BODY`, BSE SOAP XML envelopes (multi-KB) would appear in every BSE order log.

**Migration approach**: The flags stay in `httpUtil` — they control whether the HTTP wrapper calls the logger, not how the logger formats output. The structured logging migration changes **what** is logged when logging is enabled, but the flag check happens before any logging call:

```go
// Pseudocode — flag check stays the same, log call changes
if !hasFlag(opts, NO_LOG) {
    // BEFORE: GetLogger(&ctx).Info(jsonMarshaledEverything)
    // AFTER:
    logging.GetLogger(ctx).Infow(events.APICallStarted,
        fields.HTTPDomain, req.URL.Hostname(),
        fields.HTTPMethod, method,
    )
}

if !hasFlag(opts, NO_LOG_REQUEST_BODY) {
    // Log request body as a field (if needed for debugging)
}
```

**Known bug to fix**: The current `NO_LOG_REQUEST_BODY` implementation has inverted logic — it deletes the body when the flag is NOT set. This should be fixed as part of the migration.

---

## 8. Migration Guide

### Phase 1: Add Package, No Breaking Changes (Week 1)

**What**: Create `pkg/logging/` with all components. Initialize alongside existing logger.

**main.go changes:**

```go
// Existing (keep for now):
common_utils.NewAPM(config.SENTRY_DSN, config.APP_ENV)
common_utils.InitLogger(config.APP_ENV, "investments-backend", config.SENTRY_DSN)

// New (add):
logging.Init(logging.Config{
    Service:     "investments-backend",
    Environment: config.APP_ENV,
    SentryDSN:   config.SENTRY_DSN,
})
```

Both loggers run in parallel. No existing code breaks.

**Middleware changes:**
- Add `request_id` generation to APM middleware
- Add `user_id` to context in auth middleware
- These changes are additive — they don't affect existing `common_utils.GetLogger`

### Phase 2: High-Traffic Paths (Week 2)

Migrate the code paths that generate the most logs and benefit most from structure:

1. **REST APM middleware** — `events.HTTPRequestCompleted` with HTTP fields
2. **Asynq task middleware** — `events.TaskStarted/Completed/Failed` with task fields
3. **HTTP client wrapper** (`common_utils/http.go`) — `events.APICallStarted/Completed/Failed`
4. **Payment usecase** (`usecase/payments/`) — highest business value, most debugged

**Migration pattern** for a single log line:

```go
// BEFORE:
common_utils.GetLogger(&ctx).Infof("Initiating payment %s for user %s (current status: %s)",
    txnId, txn.UserID, txn.Status)

// AFTER:
logging.GetLogger(ctx).Infow(investlog.PaymentInitiated,
    investlog.PaymentID, txnId,
    investlog.PaymentStatus, txn.Status,
    investlog.PaymentMethod, txn.PaymentMethod,
)
```

**Key API difference**: `GetLogger(ctx)` takes `context.Context`, not `*context.Context`.

### Phase 3: Remaining Code (Weeks 3-4)

Migrate remaining layers PR-by-PR:
- Repository layer
- Other usecase packages (execution, dashboard, baskets, kyc, stocks)
- External service wrappers (where logging beyond httpUtil is needed)
- Asynq task handlers

### Phase 4: Cleanup (Week 5)

1. Remove `common_utils.GetLogger()` and `common_utils.InitLogger()`
2. Remove `common_utils.StandardLogger` type
3. Update all remaining callers (search for `common_utils.GetLogger`)
4. Remove the `NewLogger` function from `common_utils/logger.go`

### Backward Compatibility During Migration

During phases 1-3, both logging systems coexist:
- New code uses `logging.GetLogger(ctx)` — structured, auto-injected
- Old code uses `common_utils.GetLogger(&ctx)` — unchanged, still works
- Both write to stdout, both integrate with Sentry
- CloudWatch receives logs from both — new logs are queryable, old logs are not (but they still exist)

No big-bang migration required. Each PR converts one file/module.

#### Context Key Compatibility (Critical)

The `SentryTransactionKey` used to store Sentry spans in context **must use the same Go type** in both the old and new packages. Go's `context.Value()` matches on `(type, value)` — not just the string value.

**What breaks if we get this wrong**: During migration, some files use `common_utils.SENTRY_TRANSACTION_KEY` (type `common_utils.ContextKey`) and others use `logging.SentryTransactionKey`. If the types differ:
- Middleware writes the span with type A
- `GetLogger` tries to read with type B
- `context.Value()` returns `nil` — trace_id silently disappears from all logs
- No error, no panic — just missing correlation IDs in production

**Solution**: During Phases 1-3, the logging package imports and re-exports the existing key:

```go
// pkg/logging/context.go — Phase 1-3
import "github.com/one/backend/investments/src/packages/common_utils"
var SentryTransactionKey = common_utils.SENTRY_TRANSACTION_KEY
```

In Phase 4, when `common_utils` is removed, the key definition moves to the logging package:

```go
// pkg/logging/context.go — Phase 4 (after common_utils removal)
type contextKey string
const SentryTransactionKey contextKey = "sentry_transaction"
```

This is a single-line change in Phase 4, with all 7 files already using `logging.SentryTransactionKey` by that point.

---

## 9. CloudWatch Query Cookbook

These queries work **after migration** because every field is structured and consistently named.

### Debugging a Specific Request

```
# Find all logs for a single request (by trace_id from Sentry)
filter trace_id = "abc123def456"
| sort @timestamp asc

# Find all logs for a user in the last hour
filter user_id = "usr_789"
| sort @timestamp desc
| limit 200

# Find all logs for a specific payment
filter payment_id = "pay_abc123"
| sort @timestamp asc
```

### Error Analysis

```
# Error rate by type (last 1 hour)
filter level >= 40
| stats count(*) as errors by error.type
| sort errors desc

# Razorpay errors over time
filter error.type = "RAZORPAY_ERROR"
| stats count(*) by bin(5m)

# All gateway timeouts with details
filter error.type = "GATEWAY_TIMEOUT"
| fields @timestamp, http.domain, duration_ms, error.message, trace_id
| sort @timestamp desc
```

### Latency Analysis

```
# P99 latency per endpoint
filter msg = "http_request_completed"
| stats pct(duration_ms, 99) as p99, pct(duration_ms, 50) as p50 by http.path
| sort p99 desc

# Slow external API calls (>2 seconds)
filter msg = "api_call_completed" and duration_ms > 2000
| stats avg(duration_ms) as avg_ms, count(*) as calls by http.domain
| sort avg_ms desc

# DB query latency by collection
filter msg = "db_query_completed"
| stats avg(db.query_ms) as avg_ms, pct(db.query_ms, 99) as p99 by db.collection
| sort p99 desc
```

### Task Monitoring

```
# Failed tasks by name
filter msg = "task_failed"
| stats count(*) as failures by task.name
| sort failures desc

# Task duration by type
filter msg = "task_completed"
| stats avg(duration_ms) as avg_ms, max(duration_ms) as max_ms by task.name
| sort max_ms desc
```

### Payment Flow Analysis

```
# Payment success rate by method
filter msg in ["payment_completed", "payment_failed"]
| stats count(*) as total,
        sum(msg = "payment_completed") as success,
        sum(msg = "payment_failed") as failed
  by payment.method

# Average payment completion time
filter msg = "payment_completed"
| stats avg(duration_ms) as avg_ms by payment.method

# Payment status anomalies (FAILED → SUCCESS or SUCCESS → FAILED)
filter msg = "payment_status_anomaly"
| fields @timestamp, payment_id, payment.status, error.message, trace_id
```

### Alert-Worthy Queries

```
# ALERT: External service down (>5 errors in 5 min)
filter error.type = "SERVICE_DOWN"
| stats count(*) as errors by http.domain, bin(5m)
| filter errors > 5

# ALERT: Payment failure spike
filter msg = "payment_failed"
| stats count(*) as failures by bin(5m)
| filter failures > 20

# ALERT: DB connection errors
filter error.type = "DB_CONNECTION_ERROR"
| stats count(*) by bin(1m)
```

### Four Golden Signals Coverage

The logging package is designed to capture all four golden signals (per Google SRE) directly from structured logs. This eliminates the need for separate metric instrumentation for most operational monitoring.

| Signal | What It Measures | How Logs Capture It | Key Fields | CloudWatch Query |
|--------|-----------------|---------------------|------------|-----------------|
| **Latency** | Time to service a request/task | `duration_ms` on every request completion and task completion log | `duration_ms`, `http.path`, `task.name` | See "Latency" queries below |
| **Traffic** | Demand on the system | Count of request/task log events per time window | `msg` (event constants), `http.method`, `http.path` | See "Traffic" queries below |
| **Errors** | Rate of failed requests | `error.type` and `error.message` on error logs; HTTP status codes on request logs | `error.type`, `http.status_code`, `error.is_retryable` | See "Errors" queries below |
| **Saturation** | How "full" the system is | Queue depth logged at task start; DB/external latency as proxy for resource pressure | `queue.pending_count`, `queue.active_count`, `queue.name`, `duration_ms` | See "Saturation" queries below |

#### Latency Queries

```
# P50/P95/P99 request latency by endpoint (5-min buckets)
filter msg = "request_completed"
| stats avg(duration_ms) as p50,
        percentile(duration_ms, 95) as p95,
        percentile(duration_ms, 99) as p99
  by http.path, bin(5m)

# P95 task processing latency by task type
filter msg = "task_completed"
| stats percentile(duration_ms, 95) as p95 by task.name, bin(5m)

# Slow requests (>2s) — early warning for latency degradation
filter msg = "request_completed" and duration_ms > 2000
| stats count(*) as slow_requests by http.path, bin(5m)
```

#### Traffic Queries

```
# Requests per second by endpoint (throughput)
filter msg = "request_completed"
| stats count(*) as rps by http.path, bin(1m)

# Task throughput by type
filter msg = "task_completed" or msg = "task_failed"
| stats count(*) as tasks by task.name, bin(5m)

# Traffic trend — detect spikes or drops
filter msg = "request_completed"
| stats count(*) as total by bin(1m)
| sort @timestamp asc
```

#### Error Queries

```
# Error rate by type (for alerting)
filter ispresent(error.type)
| stats count(*) as errors by error.type, bin(5m)
| sort errors desc

# Request error rate (% of requests that failed)
filter msg in ["request_completed", "request_error"]
| stats count(*) as total,
        sum(msg = "request_error") as errors
  by bin(5m)
| display errors / total * 100 as error_pct

# Retryable vs non-retryable errors
filter ispresent(error.type)
| stats count(*) as total by error.is_retryable, bin(5m)
```

#### Saturation Queries

```
# Queue depth trend — detect growing backlogs
filter msg = "task_started" and ispresent(queue.pending_count)
| stats avg(queue.pending_count) as avg_pending,
        max(queue.pending_count) as max_pending,
        avg(queue.active_count) as avg_active
  by queue.name, bin(5m)

# ALERT: Queue saturation — pending tasks growing faster than processing
filter msg = "task_started" and ispresent(queue.pending_count)
| stats avg(queue.pending_count) as pending by bin(5m)
| filter pending > 100

# External service latency as saturation proxy (slow dependencies = backpressure)
filter msg = "api_call_completed"
| stats avg(duration_ms) as avg_latency,
        percentile(duration_ms, 99) as p99_latency
  by http.domain, bin(5m)
| filter p99_latency > 5000

# Task processing time degradation (workers saturated = tasks take longer)
filter msg = "task_completed"
| stats percentile(duration_ms, 95) as p95 by task.name, bin(5m)
| sort p95 desc
```

#### Combined Dashboard Query

```
# All 4 signals in one view (5-min buckets)
# Panel 1: Latency
filter msg = "request_completed" | stats percentile(duration_ms, 95) as latency_p95 by bin(5m)

# Panel 2: Traffic
filter msg = "request_completed" | stats count(*) as requests by bin(5m)

# Panel 3: Errors
filter ispresent(error.type) | stats count(*) as errors by bin(5m)

# Panel 4: Saturation
filter msg = "task_started" and ispresent(queue.pending_count)
| stats max(queue.pending_count) as max_queue_depth by bin(5m)
```

> **Note on Saturation**: Traditional saturation metrics (CPU, memory, disk) come from infrastructure monitoring (CloudWatch EC2/ECS metrics, Prometheus node_exporter). The logging package contributes the **application-level saturation signals** — queue depth, worker utilization, and dependency backpressure — which are often more actionable than raw infra metrics because they tell you *what* is saturated, not just *that* something is saturated.

---

## 10. Adoption Guidelines

### DO

```go
// 1. Use field constants — IDE autocomplete, compile-time safe
log := logging.GetLogger(ctx)
log.Infow(investlog.PaymentCreated,
    investlog.PaymentID, payment.ID,
    investlog.PaymentAmount, payment.Amount,
    investlog.PaymentMethod, payment.Method,
)

// 2. Use helper functions for common patterns
log.Infow(events.APICallCompleted,
    logging.APICallFields("api.razorpay.com", "POST", 200, 234)...,
)

// 3. Use MergeFields when combining helpers
log.Errorw(events.APICallFailed,
    logging.MergeFields(
        logging.APICallFields("api.razorpay.com", "POST", 504, 5234),
        logging.ErrorFields(logerrors.GatewayTimeout, err, true),
    )...,
)

// 4. Use event constants as the message (first argument)
log.Infow(events.TaskCompleted, ...)  // msg = "task_completed"

// 5. Log only relevant fields — don't dump entire structs
log.Infow(investlog.OrderPlaced,
    investlog.OrderID, order.ID,
    investlog.SchemeCode, order.SchemeCode,
    investlog.PaymentID, order.PaymentID,
)
```

### DON'T

```go
// 1. DON'T use Infof/Errorf — data becomes unqueryable
log.Infof("Payment %s created for user %s", paymentID, userID)  // BAD

// 2. DON'T use raw string field names — typos cause silent failures
log.Infow("event", "http.stauts_code", 200)  // BAD — typo

// 3. DON'T dump entire structs
log.Infow("event", "payment", payment)  // BAD — noise, possible PII

// 4. DON'T pass user_id/trace_id manually — they're auto-injected
log.Infow("event",
    fields.UserID, userID,    // UNNECESSARY — already in context
    fields.TraceID, traceID,  // UNNECESSARY — already in context
    investlog.PaymentID, id,  // This is the only field you need
)

// 5. DON'T log in loops
for _, item := range items {
    log.Infow("processing_item", "item_id", item.ID)  // BAD — log volume
}
// INSTEAD: log summary after loop
log.Infow("batch_processed", "count", len(items))

// 6. DON'T use fmt.Println — bypasses everything
fmt.Println("got config", paymentConfig.ActiveGateway)  // BAD

// 7. DON'T log PII (PAN, Aadhaar, bank account, tokens)
log.Infow("kyc_verified", "pan", user.PAN)  // BAD — security violation
// INSTEAD: log only suffix or a boolean
log.Infow("kyc_verified", "pan_suffix", user.PAN[len(user.PAN)-4:])
```

### Field Naming Convention

| Rule | Example | Wrong |
|------|---------|-------|
| `snake_case` for top-level fields | `payment_id` | `paymentId`, `PaymentID` |
| `dot.notation` for namespaced fields | `http.status_code` | `http_status_code`, `httpStatusCode` |
| No abbreviations unless universally known | `transaction_id` | `txn_id` |
| Exception: `id` suffix is always `_id` | `user_id`, `task.id` | `userId`, `taskID` |

### CI Enforcement

The codebase currently has **787 `Infof`/`Errorf` calls** vs **4 `Infow`/`Errorw` calls** (197:1 ratio). Without enforcement, new code will copy the existing pattern and the migration will never complete.

**Rule**: Block new format-string logging in CI. Existing code is not affected — only **added lines** are checked.

```yaml
# .github/workflows/lint-logging.yml
name: Lint Logging
on: [pull_request]
jobs:
  check-logging:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - name: Block new Infof/Errorf/Warnf usage
        run: |
          # Only check lines ADDED in this PR (not existing code)
          VIOLATIONS=$(git diff origin/develop...HEAD -- '*.go' \
            | grep -E '^\+' \
            | grep -v '^\+\+\+' \
            | grep -E '\.(Infof|Errorf|Warnf|Fatalf)\(' || true)

          if [ -n "$VIOLATIONS" ]; then
            echo "::error::New format-string logging detected. Use Infow/Errorw/Warnw instead."
            echo ""
            echo "Found violations:"
            echo "$VIOLATIONS"
            echo ""
            echo "Migration pattern:"
            echo '  BEFORE: log.Infof("Payment %s for user %s", id, userId)'
            echo '  AFTER:  log.Infow("payment_created", "payment_id", id)'
            exit 1
          fi
```

**Why only added lines**: We can't fix 787 calls in one PR. The CI check prevents the number from growing while migration happens file-by-file. After Phase 4 (full migration), the check becomes redundant since `common_utils.GetLogger` is removed.

**Escape hatch**: If a PR genuinely needs `Infof` (e.g., modifying old code minimally for a hotfix), the reviewer can approve with a comment noting the exception.

---

## 11. Testing Requirements

### Unit Tests for the Package

```go
// logger_test.go

func TestGetLogger_NilContext(t *testing.T) {
    Init(Config{Service: "test", Environment: "local"})
    log := GetLogger(context.Background())
    assert.NotNil(t, log)
    // Should not panic, should not include user_id/request_id
}

func TestGetLogger_WithUserID(t *testing.T) {
    Init(Config{Service: "test", Environment: "local"})
    ctx := WithUserID(context.Background(), "usr_123")
    log := GetLogger(ctx)
    // Verify user_id is in the logger's fields
    // (use an observed Zap core to capture output)
}

func TestGetLogger_NeverMutatesBase(t *testing.T) {
    Init(Config{Service: "test", Environment: "local"})

    ctx1 := WithUserID(context.Background(), "user_A")
    ctx2 := WithUserID(context.Background(), "user_B")

    log1 := GetLogger(ctx1)
    log2 := GetLogger(ctx2)

    // log1 should have user_A, log2 should have user_B
    // Neither should have the other's user ID
    // (This is the bug we're fixing from the old GetLogger)
}

func TestGetLogger_PanicsBeforeInit(t *testing.T) {
    // Reset for this test
    assert.Panics(t, func() {
        GetLogger(context.Background())
    })
}
```

```go
// helpers_test.go

func TestHTTPFields(t *testing.T) {
    f := HTTPFields("POST", "/api/v1/payments", 200, 234)
    assert.Equal(t, []interface{}{
        fields.HTTPMethod, "POST",
        fields.HTTPPath, "/api/v1/payments",
        fields.HTTPStatusCode, 200,
        fields.DurationMs, int64(234),
    }, f)
}

func TestErrorFields_NilError(t *testing.T) {
    f := ErrorFields(logerrors.GatewayTimeout, nil, true)
    assert.Equal(t, []interface{}{
        fields.ErrorType, "GATEWAY_TIMEOUT",
        fields.ErrorIsRetryable, true,
    }, f)
    // error.message should NOT be present when err is nil
}

func TestMergeFields(t *testing.T) {
    a := []interface{}{"key1", "val1"}
    b := []interface{}{"key2", "val2"}
    merged := MergeFields(a, b)
    assert.Equal(t, []interface{}{"key1", "val1", "key2", "val2"}, merged)
}
```

```go
// context_test.go

func TestWithUserID_RoundTrip(t *testing.T) {
    ctx := WithUserID(context.Background(), "usr_123")
    userID, ok := UserIDFromContext(ctx)
    assert.True(t, ok)
    assert.Equal(t, "usr_123", userID)
}

func TestUserIDFromContext_Empty(t *testing.T) {
    _, ok := UserIDFromContext(context.Background())
    assert.False(t, ok)
}
```

### Integration Test Pattern

For verifying log output in real scenarios, use Zap's `zaptest/observer`:

```go
func TestPaymentInitiateLogging(t *testing.T) {
    core, recorded := observer.New(zapcore.InfoLevel)
    // Replace baseLogger with observed core for testing
    // ...

    // Trigger the payment initiation flow
    // ...

    // Assert log output
    logs := recorded.FilterMessage("payment_initiated")
    assert.Equal(t, 1, logs.Len())

    entry := logs.All()[0]
    assert.Equal(t, "pay_123", entry.ContextMap()["payment_id"])
    assert.Equal(t, "usr_789", entry.ContextMap()["user_id"])
    assert.NotEmpty(t, entry.ContextMap()["trace_id"])
}
```

---

## 12. Appendix: Current State Audit

### Files With `fmt.Println` / `fmt.Printf` (Must Fix)

| File | Line | Code |
|------|------|------|
| `external/payments/razorpay.go` | 508 | `fmt.Println(isPaymentCreated)` |
| `external/payments/razorpay.go` | 512 | `fmt.Println("creating new order")` |
| `external/payments/razorpay.go` | 525 | `fmt.Println("creating new payment")` |
| `external/payments/razorpay.go` | 544 | `fmt.Println(err)` |
| `external/mf/bsestarmf.go` | 903 | `fmt.Printf("BuyLumpsumOrder order: %+v\n", order)` |
| `external/mf/bsestarmf.go` | 1061 | `fmt.Printf("SwitchOrder order: %+v\n", order)` |
| `usecase/kyc/kyc.go` | 4290 | `fmt.Println("Error fetching token from Redis:", err)` |
| `usecase/kyc/kyc.go` | 4293 | `fmt.Println("Fetched transaction ID from Redis:", transactionIDBytes)` |
| `usecase/config/config.go` | 66 | `fmt.Println(v.ActiveGateway, v.AssetType, v.PaymentMode)` |
| `repository/baskets/reference_baskets_mongo.go` | 103 | `fmt.Printf("%+v\n", readyMadeBasket.RebalanceNote)` |

### PII Logging Violations (Must Fix)

| File | Line | Issue |
|------|------|-------|
| `external/cloudmessaging/fcm.go` | 89 | FCM token logged in plaintext |
| `packages/common_utils/http.go` | ~178 | Request bodies logged without PII filtering |

### context.TODO() in Production (Must Fix)

| File | Line |
|------|------|
| `packages/common_utils/http.go` | 121 |

### Field Name Inconsistencies (Current State)

| Concept | Variants Found | Correct (after migration) |
|---------|---------------|--------------------------|
| User ID | `userId`, `user_id`, `userID`, `user=%s` | `user_id` (auto-injected) |
| Payment ID | `paymentId`, `payment_id`, `txnId` | `payment_id` |
| Task ID | `taskId`, `taskID` | `task.id` |
| Basket ID | `basketId`, `basket=%s` | `basket_id` |
| Error | `.With(err)`, `"error"`, inline in message | `error.type` + `error.message` |
| Duration | `"duration_ms"` (in httpUtil only) | `duration_ms` (everywhere) |

---

## Appendix: Log Output Schema

After full migration, every log line conforms to this schema:

```json
{
  "level": 20,
  "time": "2026-03-11T10:30:50.000Z",
  "msg": "<event_constant>",
  "service": "investments-backend",
  "environment": "prod",

  "trace_id": "abc123def456",
  "span_id": "789xyz",
  "user_id": "usr_789",
  "request_id": "req_456",

  "<domain_field>": "<value>",

  "error.type": "GATEWAY_TIMEOUT",
  "error.message": "context deadline exceeded",
  "error.is_retryable": true,

  "duration_ms": 1234
}
```

| Field | Source | Always Present |
|-------|--------|---------------|
| `level` | Zap level encoder | Yes |
| `time` | Zap time encoder | Yes |
| `msg` | First arg to `Infow`/`Errorw` | Yes |
| `service` | `Init(Config{Service: ...})` | Yes |
| `environment` | `Init(Config{Environment: ...})` | Yes (prod only) |
| `trace_id` | Sentry span via context | Yes (if middleware ran) |
| `span_id` | Sentry span via context | Yes (if middleware ran) |
| `user_id` | Auth middleware via context | Only for authenticated requests |
| `request_id` | APM middleware via context | Yes (if middleware ran) |
| `error.*` | `ErrorFields()` helper | Only on error logs |
| `duration_ms` | `WithDuration()` helper | Only when timing is relevant |
| Domain fields | Manual per log call | Varies |

---

*End of specification.*

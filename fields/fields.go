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
	ErrorType        = "error.type"
	ErrorMessage     = "error.message"
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
	// Values come from asynq.Inspector.GetQueueInfo() at task pickup time.
	QueuePendingCount = "queue.pending_count"
	QueueActiveCount  = "queue.active_count"
	QueueName         = "queue.name"
)

// ──────────────────────────────────────────────
// External Provider Fields
// Used in: external/ layer when calling third-party APIs
// ──────────────────────────────────────────────

const (
	ProviderName      = "provider.name"
	ProviderRequestID = "provider.request_id"
)

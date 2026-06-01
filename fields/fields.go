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
	HTTPUserAgent  = "http.user_agent"
	HTTPRequestID  = "http.request_id"
)

// ──────────────────────────────────────────────
// Error Fields
// Used in: every layer that logs errors
// ──────────────────────────────────────────────

const (
	ErrorType        = "error.type"
	ErrorMessage     = "error.message"
	ErrorIsRetryable = "error.is_retryable"
	ErrorCode        = "error.code"
	ErrorStackTrace  = "error.stack_trace"
)

// ──────────────────────────────────────────────
// Timing Fields
// Used in: middleware, external API calls, DB queries
// ──────────────────────────────────────────────

const (
	DurationMs     = "duration_ms"
	DBQueryMsTotal = "db_query_ms"
	ExternalAPIMs  = "external_api_ms"
)

// ──────────────────────────────────────────────
// Database Fields
// Used in: repository layer logging
//
// DBCollection / DBTable are aliases — pick whichever matches the store
// (Mongo "collection" vs SQL "table"). Both exist so services aren't forced
// into the wrong vocabulary.
// ──────────────────────────────────────────────

const (
	DBCollection    = "db.collection"
	DBTable         = "db.table"
	DBOperation     = "db.operation"
	DBQueryMs       = "db.query_ms"
	DBRowsAffected  = "db.rows_affected"
)

// ──────────────────────────────────────────────
// Task/Queue Fields
// Used in: asynq middleware, task handlers
// ──────────────────────────────────────────────

const (
	TaskID     = "task.id"
	TaskName   = "task.name"
	TaskType   = "task.type"
	TaskStatus = "task.status"

	// Queue saturation fields — logged by asynq middleware at task start.
	// Values come from asynq.Inspector.GetQueueInfo() at task pickup time.
	QueuePendingCount = "queue.pending_count"
	QueueActiveCount  = "queue.active_count"
	QueueDepth        = "queue.depth"
	QueueName         = "queue.name"
)

// ──────────────────────────────────────────────
// Cache Fields
// Used in: cache wrappers (Redis, in-memory)
// ──────────────────────────────────────────────

const (
	CacheHit = "cache.hit"
	CacheKey = "cache.key"
	CacheTTL = "cache.ttl_seconds"
)

// ──────────────────────────────────────────────
// Retry Fields
// Used in: retry wrappers, circuit breakers
// ──────────────────────────────────────────────

const (
	RetryAttempt  = "retry.attempt"
	RetryMaxCount = "retry.max_count"
	RetryDelayMs  = "retry.delay_ms"
)

// ──────────────────────────────────────────────
// External Provider Fields
// Used in: external/ layer when calling third-party APIs
// ──────────────────────────────────────────────

const (
	ProviderName       = "provider.name"
	ProviderRequestID  = "provider.request_id"
	ProviderResponseID = "provider.response_id"
)

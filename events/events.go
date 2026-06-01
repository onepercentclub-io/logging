package events

// ──────────────────────────────────────────────
// HTTP Lifecycle
// ──────────────────────────────────────────────

const (
	// HTTPRequestStarted is logged by the REST middleware before handling a request.
	// Fields: http.method, http.path
	HTTPRequestStarted = "http_request_started"

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
	// DBQueryStarted is logged before a DB operation begins.
	// Fields: db.collection (or db.table), db.operation
	DBQueryStarted = "db_query_started"

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

	// TaskRetrying is logged when an asynq task is being retried after a failure.
	// Fields: task.id, task.name, task.type, retry.attempt, retry.max_count, retry.delay_ms
	TaskRetrying = "task_retrying"
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
// Auth Events
// ──────────────────────────────────────────────

const (
	AuthLoginSuccess = "auth_login_success"
	AuthLoginFailed  = "auth_login_failed"
	AuthTokenRefresh = "auth_token_refresh"
	AuthTokenExpired = "auth_token_expired"
)

// ──────────────────────────────────────────────
// Cache Events
// ──────────────────────────────────────────────

const (
	CacheHit  = "cache_hit"
	CacheMiss = "cache_miss"
	CacheSet  = "cache_set"
)

// ──────────────────────────────────────────────
// Application / System Lifecycle
// ──────────────────────────────────────────────

const (
	AppStarted   = "app_started"
	AppShutdown  = "app_shutdown"
	HealthCheck  = "health_check"
	ConfigLoaded = "config_loaded"
)

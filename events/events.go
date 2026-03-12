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

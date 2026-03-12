package errors

// ──────────────────────────────────────────────
// External Service Errors
// Used when a third-party API call fails
// ──────────────────────────────────────────────

const (
	GatewayTimeout    = "GATEWAY_TIMEOUT"
	ServiceDown       = "SERVICE_DOWN"
	RateLimit         = "RATE_LIMIT"
	BadGateway        = "BAD_GATEWAY"
	ConnectionRefused = "CONNECTION_REFUSED"
)

// ──────────────────────────────────────────────
// Database Errors
// Used when a MongoDB/Redis operation fails
// ──────────────────────────────────────────────

const (
	DBConnection = "DB_CONNECTION_ERROR"
	DBQuery      = "DB_QUERY_ERROR"
	DBTimeout    = "DB_TIMEOUT"
	DBNotFound   = "DB_NOT_FOUND"
	DBDuplicate  = "DB_DUPLICATE_KEY"
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

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
	SSLError          = "SSL_ERROR"
)

// ──────────────────────────────────────────────
// Database Errors
// Used when a MongoDB/Redis/SQL operation fails
// ──────────────────────────────────────────────

const (
	DBConnection  = "DB_CONNECTION_ERROR"
	DBQuery       = "DB_QUERY_ERROR"
	DBTimeout     = "DB_TIMEOUT"
	DBNotFound    = "DB_NOT_FOUND"
	DBDuplicate   = "DB_DUPLICATE_KEY"
	DBTransaction = "DB_TRANSACTION_ERROR"
)

// ──────────────────────────────────────────────
// Validation Errors
// Used when input validation fails
// ──────────────────────────────────────────────

const (
	Validation    = "VALIDATION_ERROR"
	InvalidInput  = "INVALID_INPUT"
	MissingField  = "MISSING_FIELD"
	InvalidFormat = "INVALID_FORMAT"
)

// ──────────────────────────────────────────────
// Auth Errors
// ──────────────────────────────────────────────

const (
	Unauthorized = "UNAUTHORIZED"
	Forbidden    = "FORBIDDEN"
	TokenExpired = "TOKEN_EXPIRED"
	InvalidToken = "INVALID_TOKEN"
)

// ──────────────────────────────────────────────
// Internal Errors
// ──────────────────────────────────────────────

const (
	Internal      = "INTERNAL_ERROR"
	Panic         = "PANIC"
	ConfigError   = "CONFIG_ERROR"
	Serialization = "SERIALIZATION_ERROR"
)

// ──────────────────────────────────────────────
// Retry / Circuit Breaker Errors
// ──────────────────────────────────────────────

const (
	MaxRetriesExceeded = "MAX_RETRIES_EXCEEDED"
	CircuitOpen        = "CIRCUIT_OPEN"
)

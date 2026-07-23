package logging

import "context"

// contextKey is an unexported type for context keys defined in this package.
type contextKey string

const (
	// ctxKeyUserID stores the authenticated user's ID in context.
	// Set by: auth middleware (after JWT verification)
	// Used by: GetLogger() to auto-inject user_id
	ctxKeyUserID contextKey = "logging_user_id"

	// ctxKeyRequestID stores a unique request identifier in context.
	// Set by: APM middleware (generated UUID per request)
	// Used by: GetLogger() to auto-inject request_id
	ctxKeyRequestID contextKey = "logging_request_id"

	// ctxKeyCustomFields stores arbitrary key-value pairs in context.
	// Set by: WithField() — called in controllers/handlers to add flow context.
	// Used by: GetLogger() to auto-inject all accumulated fields.
	ctxKeyCustomFields contextKey = "logging_custom_fields"
)

// WithUserID returns a new context with the user ID set.
// Call this in auth middleware after verifying the JWT.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, userID)
}

// WithRequestID returns a new context with the request ID set.
// Call this in APM middleware at the start of each request.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKeyRequestID, requestID)
}

// WithField returns a new context with a custom field that will be auto-injected
// into every log line created via GetLogger(ctx). Fields accumulate — multiple
// WithField calls build up the set. Use this for flow-level context like
// flow_type, payment_id, basket_id, etc.
func WithField(ctx context.Context, key string, value string) context.Context {
	existing, _ := ctx.Value(ctxKeyCustomFields).(map[string]string)
	updated := make(map[string]string, len(existing)+1)
	for k, v := range existing {
		updated[k] = v
	}
	updated[key] = value
	return context.WithValue(ctx, ctxKeyCustomFields, updated)
}

// WithFields returns a new context with multiple custom fields set at once.
func WithFields(ctx context.Context, kvs map[string]string) context.Context {
	existing, _ := ctx.Value(ctxKeyCustomFields).(map[string]string)
	updated := make(map[string]string, len(existing)+len(kvs))
	for k, v := range existing {
		updated[k] = v
	}
	for k, v := range kvs {
		updated[k] = v
	}
	return context.WithValue(ctx, ctxKeyCustomFields, updated)
}

// CustomFieldsFromContext extracts all custom fields from context, if present.
func CustomFieldsFromContext(ctx context.Context) (map[string]string, bool) {
	f, ok := ctx.Value(ctxKeyCustomFields).(map[string]string)
	return f, ok && len(f) > 0
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

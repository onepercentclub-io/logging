package logging

import (
	"context"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap/zapcore"
)

// contextKey is an unexported type for context keys defined in this package.
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

// SentryTransactionKey is the context key used to store the Sentry transaction span.
//
// During service migration, middleware must write the Sentry span using this key
// so that GetLogger() can extract trace_id and span_id.
const SentryTransactionKey contextKey = "sentry_transaction"

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

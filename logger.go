package logging

import (
	"context"
	"sync"

	"github.com/TheZeroSlave/zapsentry"
	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/onepercentclub-io/logging/fields"
)

// Logger wraps Zap's SugaredLogger. Each instance is request-scoped
// and pre-populated with context fields (trace_id, user_id, etc).
// Safe for concurrent use within a single request; do NOT share across requests.
type Logger struct {
	*zap.SugaredLogger
	ctx context.Context
}

// Config holds initialization parameters.
type Config struct {
	// Service is the service name added to every log line.
	Service string

	// Environment is the deployment environment.
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
// Subsequent calls are no-ops (safe to call multiple times).
func Init(cfg Config) {
	initOnce.Do(func() {
		if cfg.Service == "" {
			panic("logging: Config.Service must not be empty")
		}

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

		// Attach Sentry core for error-level logs (non-local only).
		// The package initializes its own Sentry client from the DSN so it does not
		// depend on the service having called sentry.Init() beforehand.
		if cfg.Environment != "local" && cfg.SentryDSN != "" {
			sentryClient, sentryErr := sentry.NewClient(sentry.ClientOptions{
				Dsn:              cfg.SentryDSN,
				ServerName:       cfg.Service,
				Environment:      cfg.Environment,
				EnableTracing:    true,
				AttachStacktrace: true,
				TracesSampleRate: 1.0,
				MaxErrorDepth:    5,
				MaxBreadcrumbs:   -1,
				MaxSpans:         5,
			})
			if sentryErr == nil && sentryClient != nil {
				sentryCore, coreErr := zapsentry.NewCore(zapsentry.Configuration{
					Level:             zapcore.ErrorLevel,
					EnableBreadcrumbs: false,
					BreadcrumbLevel:   zapcore.InfoLevel,
				}, zapsentry.NewSentryClientFromClient(sentryClient))
				if coreErr == nil {
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
// This function NEVER mutates the global base logger.
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
func (l *Logger) Alertw(msg string, keysAndValues ...interface{}) {
	if l.ctx != nil {
		if spanRef := l.ctx.Value(SentryTransactionKey); spanRef != nil {
			if span, ok := spanRef.(*sentry.Span); ok {
				span.SetTag("alert", "true")
			}
		}
	}
	l.SugaredLogger.Errorw(msg, keysAndValues...)
}

// resetForTesting resets the package-level state so tests can call Init() multiple times.
// This is intentionally unexported — only test files in this package can use it.
func resetForTesting() {
	baseLogger = nil
	serviceName = ""
	initOnce = sync.Once{}
}

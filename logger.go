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
//
// The underlying SugaredLogger is intentionally unexported — callers may only
// use the structured *w methods (Infow/Errorw/...), never the format variants
// (Infof/Errorf/...). This enforces the package's "every field is queryable"
// invariant at the type level rather than via convention.
//
// Safe for concurrent use within a single request; do NOT share across requests.
type Logger struct {
	sugar *zap.SugaredLogger
	ctx   context.Context
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

	// Sampling controls log sampling to reduce CloudWatch volume.
	// If zero-valued (Sampling{}), DefaultSampling() is used in non-local environments
	// and no sampling is applied in local (so developers see every log).
	Sampling Sampling
}

var (
	baseLogger *zap.SugaredLogger
	initOnce   sync.Once
)

// integerLevelEncoder emits the numeric level used by downstream log parsers
// (CloudWatch dashboards filter on it). Zap's internal level constants are
// Debug=-1, Info=0, Warn=1, Error=2, DPanic=3, Panic=4, Fatal=5, so the
// formula (l+3)*10 produces:
//   Debug=20, Info=30, Warn=40, Error=50, DPanic=60, Panic=70, Fatal=80
// CloudWatch query for errors: `filter level = 50`.
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

		isLocal := cfg.Environment == "local"

		var zapCfg zap.Config
		if !isLocal {
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

			// Apply sampling for cost reduction. Zap's sampler thins repeated
			// log entries of the same (level, message) tuple in each 1-second
			// window: it keeps the first `Initial` entries, then 1 in every
			// `Thereafter`. Errors are always kept (see sampling.go).
			samp := cfg.Sampling
			if samp == (Sampling{}) {
				samp = DefaultSampling()
			}
			zapCfg.Sampling = samp.toZap()
		} else {
			zapCfg = zap.NewDevelopmentConfig()
			zapCfg.OutputPaths = []string{"stdout"}
			zapCfg.ErrorOutputPaths = []string{"stderr"}
			zapCfg.InitialFields = map[string]interface{}{
				fields.Service: cfg.Service,
			}
			// No sampling locally — devs need every log.
			zapCfg.Sampling = nil
		}

		zapLogger, err := zapCfg.Build()
		if err != nil {
			panic("logging: failed to build zap logger: " + err.Error())
		}

		// Attach Sentry core for error-level logs (non-local only).
		// The package initializes its own Sentry client from the DSN so it does not
		// depend on the service having called sentry.Init() beforehand.
		if !isLocal && cfg.SentryDSN != "" {
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
		return &Logger{sugar: sugar}
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

	return &Logger{sugar: sugar, ctx: ctx}
}

// Get returns a logger without context (for startup/shutdown logging).
// Prefer GetLogger(ctx) in request/task handlers.
func Get() *Logger {
	if baseLogger == nil {
		panic("logging: Init() must be called before Get()")
	}
	return &Logger{sugar: baseLogger}
}

// Debugw logs a debug-level message with structured key/value pairs.
func (l *Logger) Debugw(msg string, keysAndValues ...interface{}) {
	l.sugar.Debugw(msg, keysAndValues...)
}

// Infow logs an info-level message with structured key/value pairs.
func (l *Logger) Infow(msg string, keysAndValues ...interface{}) {
	l.sugar.Infow(msg, keysAndValues...)
}

// Warnw logs a warn-level message with structured key/value pairs.
func (l *Logger) Warnw(msg string, keysAndValues ...interface{}) {
	l.sugar.Warnw(msg, keysAndValues...)
}

// Errorw logs an error-level message with structured key/value pairs.
// In non-local environments this is also forwarded to Sentry.
func (l *Logger) Errorw(msg string, keysAndValues ...interface{}) {
	l.sugar.Errorw(msg, keysAndValues...)
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
	l.sugar.Errorw(msg, keysAndValues...)
}

// Sync flushes any buffered log entries. Call before process exit.
func (l *Logger) Sync() error {
	return l.sugar.Sync()
}

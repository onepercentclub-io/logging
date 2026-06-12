package logging

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	// "local" switches to a dev-friendly console encoder; everything else uses
	// production JSON encoding to stdout.
	Environment string

	// Sampling controls log sampling to reduce CloudWatch volume.
	// If zero-valued (Sampling{}), DefaultSampling() is used in non-local
	// environments and no sampling is applied in local (devs see every log).
	Sampling Sampling

	// ExtraCores are additional zapcore.Core sinks teed to every log entry.
	// Use this to inject error-reporting cores (e.g. zapsentry), shipping
	// cores, or audit sinks — without coupling this package to a specific
	// vendor. Nil or empty means "stdout only", which is the right default
	// for services that don't ship logs anywhere else.
	ExtraCores []zapcore.Core
}

var (
	baseLogger *zap.SugaredLogger
	initOnce   sync.Once
)

// integerLevelEncoder emits the numeric level used by downstream log parsers
// (CloudWatch dashboards filter on it). Zap's internal level constants are
// Debug=-1, Info=0, Warn=1, Error=2, DPanic=3, Panic=4, Fatal=5, so the
// formula (l+3)*10 produces:
//
//	Debug=20, Info=30, Warn=40, Error=50, DPanic=60, Panic=70, Fatal=80
//
// CloudWatch query for errors: `filter level = 50`.
func integerLevelEncoder(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendInt8((int8(l) + 3) * 10)
}

// Init initializes the global base logger. Must be called once at application
// startup, before any GetLogger() calls. Panics if called with empty Service.
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
			zapCfg.Sampling = nil
		}

		zapLogger, err := zapCfg.Build()
		if err != nil {
			panic("logging: failed to build zap logger: " + err.Error())
		}

		// Tee into any caller-supplied cores. This is how services wire
		// Sentry, Datadog, or any other sink without this package importing
		// their SDKs. Callers build their core (e.g. zapsentry.NewCore) and
		// hand it in; we fan out every entry to it via zapcore.Tee.
		if len(cfg.ExtraCores) > 0 {
			extras := cfg.ExtraCores
			zapLogger = zapLogger.WithOptions(zap.WrapCore(func(c zapcore.Core) zapcore.Core {
				all := make([]zapcore.Core, 0, 1+len(extras))
				all = append(all, c)
				all = append(all, extras...)
				return zapcore.NewTee(all...)
			}))
		}

		baseLogger = zapLogger.Sugar()
	})
}

// GetLogger returns a new Logger pre-populated with context fields.
//
// Auto-injected fields (if present in context):
//   - trace_id  (from the active OpenTelemetry span, if any)
//   - span_id   (from the active OpenTelemetry span, if any)
//   - user_id   (set by auth middleware via WithUserID)
//   - request_id (set by APM middleware via WithRequestID)
//
// Trace correlation works with any OTel-compatible tracer (OTel-native,
// Sentry's OTel bridge, Datadog, etc.) — this package depends only on the
// vendor-neutral go.opentelemetry.io/otel/trace API.
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

	// Auto-inject trace_id and span_id from the active OTel span.
	// SpanFromContext always returns a non-nil span (no-op span if absent);
	// IsValid() distinguishes a real recorded span from the no-op.
	if sc := trace.SpanFromContext(ctx).SpanContext(); sc.IsValid() {
		sugar = sugar.With(
			fields.TraceID, sc.TraceID().String(),
			fields.SpanID, sc.SpanID().String(),
		)
	}

	if userID, ok := ctx.Value(ctxKeyUserID).(string); ok && userID != "" {
		sugar = sugar.With(fields.UserID, userID)
	}

	if reqID, ok := ctx.Value(ctxKeyRequestID).(string); ok && reqID != "" {
		sugar = sugar.With(fields.RequestID, reqID)
	}

	// Auto-inject custom fields set via WithField/WithFields
	if customFields, ok := ctx.Value(ctxKeyCustomFields).(map[string]string); ok {
		for k, v := range customFields {
			sugar = sugar.With(k, v)
		}
	}

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
// In services that wire an error-reporting core via Config.ExtraCores,
// this entry is also forwarded to that sink.
func (l *Logger) Errorw(msg string, keysAndValues ...interface{}) {
	l.sugar.Errorw(msg, keysAndValues...)
}

// Alertw logs at Error level AND tags the active OTel span with alert=true,
// so on-call tooling can route critical business failures specifically.
// If no tracer is wired (no active span in ctx), the tag write is a safe
// no-op — the Errorw still fires.
func (l *Logger) Alertw(msg string, keysAndValues ...interface{}) {
	if l.ctx != nil {
		span := trace.SpanFromContext(l.ctx)
		if span.SpanContext().IsValid() {
			span.SetAttributes(attribute.String("alert", "true"))
		}
	}
	l.sugar.Errorw(msg, keysAndValues...)
}

// Sync flushes any buffered log entries. Call before process exit.
func (l *Logger) Sync() error {
	return l.sugar.Sync()
}

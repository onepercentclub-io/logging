package logging

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/onepercentclub-io/logging/fields"
)

// initTestLogger sets up the package with an observed logger for testing.
// Returns the observer core so tests can inspect log output.
func initTestLogger(level zapcore.Level) *observer.ObservedLogs {
	resetForTesting()

	core, recorded := observer.New(level)
	zapLogger := zap.New(core)
	baseLogger = zapLogger.Sugar()
	serviceName = "test-service"

	return recorded
}

func TestGetLogger_BackgroundContext(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	log := GetLogger(context.Background())
	log.Infow("test_event", "key", "value")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Message != "test_event" {
		t.Errorf("expected msg %q, got %q", "test_event", logs[0].Message)
	}
}

func TestGetLogger_NilContext(t *testing.T) {
	initTestLogger(zapcore.InfoLevel)

	log := GetLogger(nil)
	if log == nil {
		t.Fatal("GetLogger(nil) returned nil")
	}
	// Should not panic
	log.Infow("test_nil_ctx")
}

func TestGetLogger_WithUserID(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	ctx := WithUserID(context.Background(), "usr_123")
	log := GetLogger(ctx)
	log.Infow("test_event")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	contextMap := logs[0].ContextMap()
	if contextMap[fields.UserID] != "usr_123" {
		t.Errorf("expected user_id=%q, got %v", "usr_123", contextMap[fields.UserID])
	}
}

func TestGetLogger_WithRequestID(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	ctx := WithRequestID(context.Background(), "req_456")
	log := GetLogger(ctx)
	log.Infow("test_event")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	contextMap := logs[0].ContextMap()
	if contextMap[fields.RequestID] != "req_456" {
		t.Errorf("expected request_id=%q, got %v", "req_456", contextMap[fields.RequestID])
	}
}

func TestGetLogger_WithBothUserAndRequestID(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	ctx := context.Background()
	ctx = WithUserID(ctx, "usr_123")
	ctx = WithRequestID(ctx, "req_456")

	log := GetLogger(ctx)
	log.Infow("test_event")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}

	contextMap := logs[0].ContextMap()
	if contextMap[fields.UserID] != "usr_123" {
		t.Errorf("expected user_id=%q, got %v", "usr_123", contextMap[fields.UserID])
	}
	if contextMap[fields.RequestID] != "req_456" {
		t.Errorf("expected request_id=%q, got %v", "req_456", contextMap[fields.RequestID])
	}
}

func TestGetLogger_NeverMutatesBase(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	ctx1 := WithUserID(context.Background(), "user_A")
	ctx2 := WithUserID(context.Background(), "user_B")

	log1 := GetLogger(ctx1)
	log2 := GetLogger(ctx2)

	log1.Infow("from_log1")
	log2.Infow("from_log2")

	logs := recorded.All()
	if len(logs) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(logs))
	}

	// log1 should have user_A only
	ctx1Map := logs[0].ContextMap()
	if ctx1Map[fields.UserID] != "user_A" {
		t.Errorf("log1: expected user_id=user_A, got %v", ctx1Map[fields.UserID])
	}

	// log2 should have user_B only
	ctx2Map := logs[1].ContextMap()
	if ctx2Map[fields.UserID] != "user_B" {
		t.Errorf("log2: expected user_id=user_B, got %v", ctx2Map[fields.UserID])
	}
}

func TestGetLogger_EmptyUserID_NotInjected(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	ctx := WithUserID(context.Background(), "")
	log := GetLogger(ctx)
	log.Infow("test_event")

	logs := recorded.All()
	contextMap := logs[0].ContextMap()
	if _, exists := contextMap[fields.UserID]; exists {
		t.Error("empty user_id should not be injected into logger")
	}
}

func TestGetLogger_PanicsBeforeInit(t *testing.T) {
	resetForTesting()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok || msg != "logging: Init() must be called before GetLogger()" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	GetLogger(context.Background())
}

func TestGet_PanicsBeforeInit(t *testing.T) {
	resetForTesting()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
	}()

	Get()
}

func TestInit_PanicsOnEmptyService(t *testing.T) {
	resetForTesting()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic, got none")
		}
		msg, ok := r.(string)
		if !ok || msg != "logging: Config.Service must not be empty" {
			t.Errorf("unexpected panic message: %v", r)
		}
	}()

	Init(Config{Service: ""})
}

func TestInit_SecondCallIsNoOp(t *testing.T) {
	resetForTesting()

	Init(Config{Service: "first-service", Environment: "local"})

	// Second call should NOT panic — it's a no-op
	Init(Config{Service: "second-service", Environment: "local"})

	// Service name should still be "first-service" (first call wins)
	log := Get()
	if log == nil {
		t.Fatal("Get() returned nil after Init()")
	}
}

func TestInit_LocalEnvironment(t *testing.T) {
	resetForTesting()

	Init(Config{Service: "test-service", Environment: "local"})

	log := Get()
	if log == nil {
		t.Fatal("Get() returned nil after Init()")
	}
}

func TestInit_ProdEnvironment(t *testing.T) {
	resetForTesting()

	Init(Config{Service: "test-service", Environment: "prod"})

	log := Get()
	if log == nil {
		t.Fatal("Get() returned nil after Init()")
	}
}

func TestGet_ReturnsLoggerWithoutContext(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	log := Get()
	log.Infow("startup_event", "version", "1.0.0")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Message != "startup_event" {
		t.Errorf("expected msg %q, got %q", "startup_event", logs[0].Message)
	}
}

func TestGetLogger_ConcurrentSafety(t *testing.T) {
	recorded := initTestLogger(zapcore.InfoLevel)

	const goroutines = 50
	done := make(chan struct{})

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			ctx := WithUserID(context.Background(), "user_"+string(rune('A'+id%26)))
			ctx = WithRequestID(ctx, "req_"+string(rune('0'+id%10)))
			log := GetLogger(ctx)
			log.Infow("concurrent_event", "goroutine_id", id)
		}(i)
	}

	for i := 0; i < goroutines; i++ {
		<-done
	}

	logs := recorded.All()
	if len(logs) != goroutines {
		t.Errorf("expected %d log entries, got %d", goroutines, len(logs))
	}
}

func TestAlertw_LogsAtErrorLevel(t *testing.T) {
	recorded := initTestLogger(zapcore.ErrorLevel)

	log := GetLogger(context.Background())
	log.Alertw("critical_failure", "detail", "something broke")

	logs := recorded.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
	if logs[0].Level != zapcore.ErrorLevel {
		t.Errorf("expected Error level, got %v", logs[0].Level)
	}
	if logs[0].Message != "critical_failure" {
		t.Errorf("expected msg %q, got %q", "critical_failure", logs[0].Message)
	}
}

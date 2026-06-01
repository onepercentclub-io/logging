// example is a runnable dev harness that exercises every feature of the
// logging package end-to-end. It pretends to be a small HTTP service that
// receives a request, calls an external API, hits the DB, runs a cache
// lookup, retries on failure, and finally enqueues an async task.
//
// Run with:
//   go run ./example/        # local mode, dev-friendly console output
//   ENV=prod go run ./example/  # production JSON output (one log per line)
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onepercentclub-io/logging"
	logerrors "github.com/onepercentclub-io/logging/errors"
	"github.com/onepercentclub-io/logging/events"
	"github.com/onepercentclub-io/logging/fields"
)

func main() {
	env := os.Getenv("ENV")
	if env == "" {
		env = "local"
	}

	// ── 1. Initialize once at startup ──────────────────────────────────────
	logging.Init(logging.Config{
		Service:     "investments-backend",
		Environment: env,
		// SentryDSN intentionally empty — we don't want to actually ship
		// events to Sentry from the example. In a real service you'd pass
		// os.Getenv("SENTRY_DSN") here.
	})

	startup := logging.Get()
	startup.Infow(events.AppStarted, "version", "1.0.0", "pid", os.Getpid())
	defer func() {
		startup.Infow(events.AppShutdown)
		_ = startup.Sync()
	}()

	section("Simulating an HTTP request lifecycle")
	simulateHTTPRequest()

	section("Simulating an external API call that fails")
	simulateAPIFailure()

	section("Simulating a DB query that succeeds")
	simulateDBQuery()

	section("Simulating a cache hit + miss")
	simulateCache()

	section("Simulating a retry loop")
	simulateRetry()

	section("Simulating an async task lifecycle")
	simulateTask()

	section("Critical failure → Alertw")
	simulateAlert()

	section("Context isolation between two requests")
	simulateContextIsolation()

	section("Extracting values back from context")
	demoExtraction()

	section("Done — scroll up to inspect output")
}

// ── Scenario 1: HTTP request ────────────────────────────────────────────────
// In real code this is what the REST middleware would emit.
func simulateHTTPRequest() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	start := time.Now()
	// ... handler does work ...
	time.Sleep(15 * time.Millisecond)

	log.Infow(events.HTTPRequestCompleted,
		logging.MergeFields(
			logging.HTTPFields("POST", "/api/v1/payments", 200, time.Since(start).Milliseconds()),
		)...,
	)
}

// ── Scenario 2: failed external API call ────────────────────────────────────
// Exercises APICallFields + ErrorFields + MergeFields composition.
func simulateAPIFailure() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	err := errors.New("context deadline exceeded")
	log.Errorw(events.APICallFailed,
		logging.MergeFields(
			logging.APICallFields("api.razorpay.com", "POST", 504, 5234),
			logging.ErrorFields(logerrors.GatewayTimeout, err, true),
			[]interface{}{fields.ProviderName, "razorpay"},
		)...,
	)
}

// ── Scenario 3: DB query ────────────────────────────────────────────────────
func simulateDBQuery() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	log.Infow(events.DBQueryCompleted,
		logging.DBFields("payments", "find_one", 45)...,
	)

	// A failure too, so we see error fields composed with DB fields
	log.Errorw(events.DBQueryFailed,
		logging.MergeFields(
			logging.DBFields("payments", "insert", 120),
			logging.ErrorFields(logerrors.DBDuplicate, errors.New("E11000 duplicate key"), false),
		)...,
	)
}

// ── Scenario 4: cache hit / miss ────────────────────────────────────────────
func simulateCache() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	log.Infow(events.CacheHit, logging.CacheFields("user:usr_789:profile", true, 300)...)
	log.Infow(events.CacheMiss, logging.CacheFields("user:usr_789:portfolio", false, 0)...)
}

// ── Scenario 5: retry loop ──────────────────────────────────────────────────
func simulateRetry() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Pretend the call still fails — log a retry attempt
		log.Warnw(events.TaskRetrying,
			logging.MergeFields(
				logging.RetryFields(attempt, maxAttempts, int64(attempt*100)),
				logging.ErrorFields(logerrors.GatewayTimeout, errors.New("upstream still slow"), true),
			)...,
		)
	}

	// And finally the give-up log
	log.Errorw("retry_exhausted",
		logging.ErrorFields(logerrors.MaxRetriesExceeded, errors.New("3 attempts failed"), false)...,
	)
}

// ── Scenario 6: async task lifecycle ────────────────────────────────────────
// Mirrors what the asynq middleware would emit.
func simulateTask() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	start := time.Now()
	taskID, taskName, taskType := "task_001", "process_payment", "asynq"

	log.Infow(events.TaskStarted,
		logging.MergeFields(
			logging.TaskFields(taskID, taskName, taskType),
			logging.QueueFields("default", 12, 3),
		)...,
	)

	time.Sleep(20 * time.Millisecond) // pretend to do work

	log.Infow(events.TaskCompleted,
		logging.MergeFields(
			logging.TaskFields(taskID, taskName, taskType),
			logging.WithDuration(start),
		)...,
	)
}

// ── Scenario 7: critical alert ──────────────────────────────────────────────
func simulateAlert() {
	ctx := newRequestContext("usr_789", "req_abc-123")
	log := logging.GetLogger(ctx)

	log.Alertw("sip_execution_failed",
		fields.ErrorType, logerrors.Internal,
		fields.ErrorMessage, "execution plan not found",
		"sip_config_id", "sip_xyz",
	)
}

// ── Scenario 8: context isolation ───────────────────────────────────────────
// The whole point of GetLogger(ctx) returning a fresh logger: two concurrent
// requests cannot leak fields into each other.
func simulateContextIsolation() {
	logA := logging.GetLogger(newRequestContext("usr_AAA", "req_AAA"))
	logB := logging.GetLogger(newRequestContext("usr_BBB", "req_BBB"))

	logA.Infow("isolation_check_a", "note", "should show usr_AAA only")
	logB.Infow("isolation_check_b", "note", "should show usr_BBB only")
}

// ── Scenario 9: pull values back out ────────────────────────────────────────
func demoExtraction() {
	ctx := newRequestContext("usr_789", "req_abc-123")

	if uid, ok := logging.UserIDFromContext(ctx); ok {
		fmt.Printf("  UserIDFromContext   → %s\n", uid)
	}
	if rid, ok := logging.RequestIDFromContext(ctx); ok {
		fmt.Printf("  RequestIDFromContext → %s\n", rid)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

// newRequestContext is what middleware would do: stash user_id + request_id
// onto the context so every downstream log picks them up automatically.
func newRequestContext(userID, requestID string) context.Context {
	ctx := context.Background()
	ctx = logging.WithUserID(ctx, userID)
	ctx = logging.WithRequestID(ctx, requestID)
	return ctx
}

func section(title string) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 70))
	fmt.Println("▸ " + title)
	fmt.Println(strings.Repeat("─", 70))
}

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/onepercentclub-io/logging"
	"github.com/onepercentclub-io/logging/events"
	logerrors "github.com/onepercentclub-io/logging/errors"
	"github.com/onepercentclub-io/logging/fields"
)

func main() {
	// 1. Initialize the logger (done once at startup)
	logging.Init(logging.Config{
		Service:     "investments-backend",
		Environment: "local",
	})

	fmt.Println("=== Startup logging (no context) ===")
	log := logging.Get()
	log.Infow(events.AppStarted, "version", "1.0.0")

	// 2. Simulate a request with user_id and request_id in context
	fmt.Println("\n=== Request with context ===")
	ctx := context.Background()
	ctx = logging.WithUserID(ctx, "usr_789")
	ctx = logging.WithRequestID(ctx, "req_abc-123")

	rlog := logging.GetLogger(ctx)
	rlog.Infow(events.HTTPRequestCompleted,
		logging.HTTPFields("POST", "/api/v1/payments", 200, 234)...,
	)

	// 3. Simulate an external API call failure
	fmt.Println("\n=== API call failure ===")
	err := errors.New("context deadline exceeded")
	rlog.Errorw(events.APICallFailed,
		logging.MergeFields(
			logging.APICallFields("api.razorpay.com", "POST", 504, 5234),
			logging.ErrorFields(logerrors.GatewayTimeout, err, true),
		)...,
	)

	// 4. Simulate a DB query
	fmt.Println("\n=== DB query ===")
	rlog.Infow(events.DBQueryCompleted,
		logging.DBFields("payments", "find_one", 45)...,
	)

	// 5. Simulate a task
	fmt.Println("\n=== Task processing ===")
	rlog.Infow(events.TaskStarted,
		logging.TaskFields("task_001", "process_payment", "asynq")...,
	)

	// 6. Show that different contexts don't leak
	fmt.Println("\n=== Context isolation ===")
	ctx2 := logging.WithUserID(context.Background(), "usr_OTHER")
	log2 := logging.GetLogger(ctx2)
	log2.Infow("isolated_event", "note", "this should have usr_OTHER, not usr_789")

	// 7. Alertw
	fmt.Println("\n=== Alert ===")
	rlog.Alertw("sip_execution_failed",
		fields.ErrorType, logerrors.Internal,
		fields.ErrorMessage, "execution plan not found",
	)

	// 8. Extract context values back
	fmt.Println("\n=== Context extraction ===")
	if uid, ok := logging.UserIDFromContext(ctx); ok {
		fmt.Printf("UserID from context: %s\n", uid)
	}
	if rid, ok := logging.RequestIDFromContext(ctx); ok {
		fmt.Printf("RequestID from context: %s\n", rid)
	}

	fmt.Println("\n=== Done ===")
}

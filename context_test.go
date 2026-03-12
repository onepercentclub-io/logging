package logging

import (
	"context"
	"testing"
)

func TestWithUserID_RoundTrip(t *testing.T) {
	ctx := WithUserID(context.Background(), "usr_123")
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		t.Fatal("UserIDFromContext: expected ok=true")
	}
	if userID != "usr_123" {
		t.Errorf("UserIDFromContext: expected %q, got %q", "usr_123", userID)
	}
}

func TestUserIDFromContext_Empty(t *testing.T) {
	_, ok := UserIDFromContext(context.Background())
	if ok {
		t.Error("UserIDFromContext: expected ok=false for empty context")
	}
}

func TestUserIDFromContext_EmptyString(t *testing.T) {
	ctx := WithUserID(context.Background(), "")
	_, ok := UserIDFromContext(ctx)
	if ok {
		t.Error("UserIDFromContext: expected ok=false for empty string user ID")
	}
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req_456")
	reqID, ok := RequestIDFromContext(ctx)
	if !ok {
		t.Fatal("RequestIDFromContext: expected ok=true")
	}
	if reqID != "req_456" {
		t.Errorf("RequestIDFromContext: expected %q, got %q", "req_456", reqID)
	}
}

func TestRequestIDFromContext_Empty(t *testing.T) {
	_, ok := RequestIDFromContext(context.Background())
	if ok {
		t.Error("RequestIDFromContext: expected ok=false for empty context")
	}
}

func TestWithUserID_DoesNotOverwriteRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = WithRequestID(ctx, "req_456")
	ctx = WithUserID(ctx, "usr_123")

	userID, ok := UserIDFromContext(ctx)
	if !ok || userID != "usr_123" {
		t.Errorf("expected user_id=usr_123, got %q (ok=%v)", userID, ok)
	}

	reqID, ok := RequestIDFromContext(ctx)
	if !ok || reqID != "req_456" {
		t.Errorf("expected request_id=req_456, got %q (ok=%v)", reqID, ok)
	}
}

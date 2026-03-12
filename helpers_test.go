package logging

import (
	"errors"
	"testing"
	"time"

	"github.com/onepercentclub-io/logging/fields"
)

func TestHTTPFields(t *testing.T) {
	f := HTTPFields("POST", "/api/v1/payments", 200, 234)
	expected := []interface{}{
		fields.HTTPMethod, "POST",
		fields.HTTPPath, "/api/v1/payments",
		fields.HTTPStatusCode, 200,
		fields.DurationMs, int64(234),
	}
	assertFieldsEqual(t, expected, f)
}

func TestAPICallFields(t *testing.T) {
	f := APICallFields("api.razorpay.com", "POST", 504, 5234)
	expected := []interface{}{
		fields.HTTPDomain, "api.razorpay.com",
		fields.HTTPMethod, "POST",
		fields.HTTPStatusCode, 504,
		fields.DurationMs, int64(5234),
	}
	assertFieldsEqual(t, expected, f)
}

func TestErrorFields_WithError(t *testing.T) {
	err := errors.New("context deadline exceeded")
	f := ErrorFields("GATEWAY_TIMEOUT", err, true)
	expected := []interface{}{
		fields.ErrorType, "GATEWAY_TIMEOUT",
		fields.ErrorIsRetryable, true,
		fields.ErrorMessage, "context deadline exceeded",
	}
	assertFieldsEqual(t, expected, f)
}

func TestErrorFields_NilError(t *testing.T) {
	f := ErrorFields("GATEWAY_TIMEOUT", nil, true)
	expected := []interface{}{
		fields.ErrorType, "GATEWAY_TIMEOUT",
		fields.ErrorIsRetryable, true,
	}
	assertFieldsEqual(t, expected, f)

	// error.message should NOT be present when err is nil
	for i := 0; i < len(f); i += 2 {
		if f[i] == fields.ErrorMessage {
			t.Error("ErrorFields should not include error.message when err is nil")
		}
	}
}

func TestDBFields(t *testing.T) {
	f := DBFields("payments", "find_one", 45)
	expected := []interface{}{
		fields.DBCollection, "payments",
		fields.DBOperation, "find_one",
		fields.DBQueryMs, int64(45),
	}
	assertFieldsEqual(t, expected, f)
}

func TestTaskFields(t *testing.T) {
	f := TaskFields("task_123", "process_payment", "asynq")
	expected := []interface{}{
		fields.TaskID, "task_123",
		fields.TaskName, "process_payment",
		fields.TaskType, "asynq",
	}
	assertFieldsEqual(t, expected, f)
}

func TestWithDuration(t *testing.T) {
	start := time.Now().Add(-100 * time.Millisecond)
	f := WithDuration(start)

	if len(f) != 2 {
		t.Fatalf("WithDuration: expected 2 elements, got %d", len(f))
	}
	if f[0] != fields.DurationMs {
		t.Errorf("WithDuration: expected key %q, got %q", fields.DurationMs, f[0])
	}
	ms, ok := f[1].(int64)
	if !ok {
		t.Fatalf("WithDuration: expected int64 value, got %T", f[1])
	}
	if ms < 90 || ms > 500 {
		t.Errorf("WithDuration: expected ~100ms, got %dms", ms)
	}
}

func TestMergeFields(t *testing.T) {
	a := []interface{}{"key1", "val1"}
	b := []interface{}{"key2", "val2"}
	merged := MergeFields(a, b)
	expected := []interface{}{"key1", "val1", "key2", "val2"}
	assertFieldsEqual(t, expected, merged)
}

func TestMergeFields_Empty(t *testing.T) {
	merged := MergeFields()
	if len(merged) != 0 {
		t.Errorf("MergeFields(): expected empty slice, got %v", merged)
	}
}

func TestMergeFields_Single(t *testing.T) {
	a := []interface{}{"key1", "val1"}
	merged := MergeFields(a)
	assertFieldsEqual(t, a, merged)
}

// assertFieldsEqual is a test helper that compares two []interface{} slices.
func assertFieldsEqual(t *testing.T, expected, actual []interface{}) {
	t.Helper()
	if len(expected) != len(actual) {
		t.Fatalf("length mismatch: expected %d, got %d\nexpected: %v\nactual:   %v",
			len(expected), len(actual), expected, actual)
	}
	for i := range expected {
		if expected[i] != actual[i] {
			t.Errorf("index %d: expected %v (%T), got %v (%T)",
				i, expected[i], expected[i], actual[i], actual[i])
		}
	}
}

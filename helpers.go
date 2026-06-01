package logging

import (
	"time"

	"github.com/onepercentclub-io/logging/fields"
)

// HTTPFields returns structured fields for an HTTP request log.
func HTTPFields(method, path string, statusCode int, durationMs int64) []interface{} {
	return []interface{}{
		fields.HTTPMethod, method,
		fields.HTTPPath, path,
		fields.HTTPStatusCode, statusCode,
		fields.DurationMs, durationMs,
	}
}

// APICallFields returns structured fields for an external API call log.
func APICallFields(domain, method string, statusCode int, durationMs int64) []interface{} {
	return []interface{}{
		fields.HTTPDomain, domain,
		fields.HTTPMethod, method,
		fields.HTTPStatusCode, statusCode,
		fields.DurationMs, durationMs,
	}
}

// ErrorFields returns structured fields for error logging.
// error.message is omitted when err is nil so callers can pass a nil error
// alongside a known error.type without polluting the log.
func ErrorFields(errorType string, err error, isRetryable bool) []interface{} {
	f := []interface{}{
		fields.ErrorType, errorType,
		fields.ErrorIsRetryable, isRetryable,
	}
	if err != nil {
		f = append(f, fields.ErrorMessage, err.Error())
	}
	return f
}

// DBFields returns structured fields for a database operation log.
// `collection` maps to fields.DBCollection; use this name for both Mongo
// collections and SQL tables — pick whichever vocabulary fits the service.
func DBFields(collection, operation string, queryMs int64) []interface{} {
	return []interface{}{
		fields.DBCollection, collection,
		fields.DBOperation, operation,
		fields.DBQueryMs, queryMs,
	}
}

// TaskFields returns structured fields for an asynq task log.
func TaskFields(taskID, taskName, taskType string) []interface{} {
	return []interface{}{
		fields.TaskID, taskID,
		fields.TaskName, taskName,
		fields.TaskType, taskType,
	}
}

// QueueFields returns structured fields for queue saturation logging,
// typically emitted by the asynq middleware at task pickup.
func QueueFields(queueName string, pending, active int) []interface{} {
	return []interface{}{
		fields.QueueName, queueName,
		fields.QueuePendingCount, pending,
		fields.QueueActiveCount, active,
	}
}

// CacheFields returns structured fields for a cache operation log.
func CacheFields(key string, hit bool, ttlSeconds int) []interface{} {
	return []interface{}{
		fields.CacheKey, key,
		fields.CacheHit, hit,
		fields.CacheTTL, ttlSeconds,
	}
}

// RetryFields returns structured fields for a retry attempt log.
func RetryFields(attempt, maxCount int, delayMs int64) []interface{} {
	return []interface{}{
		fields.RetryAttempt, attempt,
		fields.RetryMaxCount, maxCount,
		fields.RetryDelayMs, delayMs,
	}
}

// WithDuration returns a duration_ms field computed from a start time.
func WithDuration(start time.Time) []interface{} {
	return []interface{}{
		fields.DurationMs, time.Since(start).Milliseconds(),
	}
}

// MergeFields concatenates multiple field slices into one.
func MergeFields(fieldSets ...[]interface{}) []interface{} {
	var total int
	for _, f := range fieldSets {
		total += len(f)
	}
	merged := make([]interface{}, 0, total)
	for _, f := range fieldSets {
		merged = append(merged, f...)
	}
	return merged
}

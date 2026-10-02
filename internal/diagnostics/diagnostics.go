// Package diagnostics defines stable public error codes and request correlation.
package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/willibrandon/mtlog/core"
)

const RequestIDMeta = "io.github.smrgd88.pixel-mcp/request_id"
const ErrorMeta = "io.github.smrgd88.pixel-mcp/error"

type Error struct {
	Code  string
	Cause error
}

func (e *Error) Error() string     { return e.Cause.Error() }
func (e *Error) Unwrap() error     { return e.Cause }
func (e *Error) ErrorCode() string { return e.Code }
func Wrap(code string, err error) error {
	if err == nil {
		return nil
	}
	return &Error{code, err}
}
func Errorf(code, format string, args ...any) error { return Wrap(code, fmt.Errorf(format, args...)) }

type PublicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var messages = map[string]string{
	"invalid_arguments":          "Invalid tool arguments; check the tool input schema and parameter limits.",
	"not_found":                  "The requested file or resource was not found.",
	"permission_denied":          "The operation does not have the required file permissions.",
	"timeout":                    "The operation exceeded its deadline.",
	"cancelled":                  "The operation was cancelled.",
	"aseprite_execution_failed":  "Aseprite could not complete the command.",
	"lua_error":                  "The Aseprite Lua operation failed.",
	"unsupported_aseprite":       "The Aseprite version or API is below the supported minimum.",
	"capability_probe_failed":    "The Aseprite runtime could not be inspected.",
	"file_changed":               "The file changed during the operation; inspect it before retrying.",
	"file_commit_failed":         "The file replacement failed.",
	"file_lock_scope":            "The operation could not reserve the required file locks.",
	"snapshot_invalid":           "The snapshot request or stored snapshot is invalid.",
	"snapshot_capacity":          "Snapshot storage is full; delete an unneeded snapshot before retrying.",
	"snapshot_expired":           "The snapshot has expired.",
	"snapshot_source_mismatch":   "The snapshot belongs to another source file.",
	"history_invalid":            "The history request or stored history is invalid.",
	"history_scope":              "History cannot be recorded inside an existing sprite transaction.",
	"history_conflict":           "The file has changed since the recorded operation.",
	"history_operation_mismatch": "The expected operation is not the latest applied operation; refresh history.",
	"history_recovery_required":  "History has an unresolved transition; inspect the saved snapshots before proceeding.",
	"operation_failed":           "The tool operation failed.",
	"protocol_error":             "The tool request could not be processed.",
}

// Classify never parses error text or returns paths, arguments, stderr or Lua.
func Classify(err error) PublicError {
	code := "operation_failed"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code = "timeout"
	case errors.Is(err, context.Canceled):
		code = "cancelled"
	default:
		var coded interface{ ErrorCode() string }
		if errors.As(err, &coded) {
			code = coded.ErrorCode()
		} else if errors.Is(err, os.ErrNotExist) {
			code = "not_found"
		} else if errors.Is(err, os.ErrPermission) {
			code = "permission_denied"
		}
	}
	return Public(code)
}
func Public(code string) PublicError {
	message, ok := messages[code]
	if !ok {
		code = "operation_failed"
		message = messages[code]
	}
	return PublicError{code, message}
}

type requestIDKey struct{}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}
func RequestID(ctx context.Context) string { id, _ := ctx.Value(requestIDKey{}).(string); return id }

// RedactLogFilter sanitizes the bundled CLI's non-debug structured logs before
// either console or file sinks see them. Message templates are static in our code.
type RedactLogFilter struct{}

func (RedactLogFilter) IsEnabled(event *core.LogEvent) bool {
	for name := range event.Properties {
		switch name {
		case "RequestID", "SourceContext", "Tool", "Duration", "Outcome", "ErrorCode":
		default:
			event.Properties[name] = "[redacted]"
		}
	}
	event.Exception = nil
	return true
}

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog/core"
	"os"
	"testing"
)

func TestClassifyTypedCausesAndNoTextGuessing(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{Errorf("invalid_arguments", "bad secret %q", "/private/name"), "invalid_arguments"},
		{fmt.Errorf("wrapped: %w", Errorf("history_conflict", "secret")), "history_conflict"},
		{Wrap("capability_probe_failed", context.DeadlineExceeded), "timeout"},
		{Wrap("lua_error", context.Canceled), "cancelled"},
		{fmt.Errorf("read: %w", os.ErrNotExist), "not_found"},
		{os.ErrPermission, "permission_denied"},
		{errors.New("snapshot_capacity: user supplied text"), "operation_failed"},
		{Errorf("attacker_code", "secret"), "operation_failed"},
	} {
		out := Classify(tc.err)
		require.Equal(t, tc.code, out.Code)
		require.NotContains(t, out.Message, "secret")
		require.NotContains(t, out.Message, "/private")
	}
}
func TestRedactLogFilter(t *testing.T) {
	e := &core.LogEvent{MessageTemplate: "failure {Error} {Config} {RequestID}", Properties: map[string]any{"Error": errors.New("secret"), "Config": "/private/name", "RequestID": "id"}, Exception: errors.New("sensitive")}
	require.True(t, (RedactLogFilter{}).IsEnabled(e))
	require.Nil(t, e.Exception)
	require.NotContains(t, e.RenderMessage(), "secret")
	require.NotContains(t, e.RenderMessage(), "/private")
	require.Contains(t, e.RenderMessage(), "id")
}

func TestRollbackFailureOutranksJoinedCauses(t *testing.T) {
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, Errorf("file_commit_failed", "write failed")} {
		ref := RecoveryReference{OutputIndex: 1, Directory: ".pixel-mcp-stage-safe", BackupFile: ".original-backup", RollbackFailed: true}
		rollback := &RollbackError{Cause: errors.Join(cause, errors.New("raw private recovery path")), Recovery: []RecoveryReference{ref}}
		out := Classify(fmt.Errorf("outer: %w", errors.Join(cause, rollback)))
		require.Equal(t, "file_rollback_failed", out.Code)
		require.Equal(t, []RecoveryReference{ref}, out.Recovery)
		require.NotContains(t, out.Message, "private")
		require.ErrorIs(t, rollback, cause)
		out.Recovery[0].Directory = "changed"
		require.Equal(t, ".pixel-mcp-stage-safe", rollback.Recovery[0].Directory)
	}
}

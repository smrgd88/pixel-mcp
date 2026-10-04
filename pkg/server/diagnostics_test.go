package server

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/sinks"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
)

func diagnosticSession(t *testing.T, timing bool) (*mcp.ClientSession, *sinks.MemorySink) {
	t.Helper()
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	cfg.SnapshotDir = filepath.Join(t.TempDir(), "snapshots")
	cfg.EnableTiming = timing
	sink := sinks.NewMemorySink()
	logger := mtlog.New(mtlog.WithSink(sink), mtlog.WithFilter(diagnostics.RedactLogFilter{}))
	s, err := New(cfg, logger)
	require.NoError(t, err)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.mcp.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return cs, sink
}
func TestToolDiagnosticsValidationAndCorrelation(t *testing.T) {
	for _, timing := range []bool{false, true} {
		cs, sink := diagnosticSession(t, timing)
		res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Meta: mcp.Meta{diagnostics.RequestIDMeta: "client-injected-id"}, Name: "create_canvas", Arguments: map[string]any{"width": 8, "height": 8, "color_mode": "secret-path-invalid"}})
		require.NoError(t, err)
		require.True(t, res.IsError)
		id, ok := res.Meta[diagnostics.RequestIDMeta].(string)
		require.True(t, ok)
		require.NotEmpty(t, id)
		require.NotEqual(t, "client-injected-id", id)
		encoded, err := json.Marshal(res)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "secret-path-invalid")
		var payload struct {
			RequestID string                  `json:"request_id"`
			Error     diagnostics.PublicError `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(res.Content[0].(*mcp.TextContent).Text), &payload))
		require.Equal(t, id, payload.RequestID)
		require.Equal(t, "invalid_arguments", payload.Error.Code)
		found := false
		for _, event := range sink.Events() {
			require.NotContains(t, event.RenderMessage(), "secret-path-invalid")
			if event.Properties["RequestID"] == id {
				found = true
			}
			if v, ok := event.Properties["RequestID"]; ok {
				require.Equal(t, id, v)
			}
		}
		require.True(t, found)
	}
}
func TestToolDiagnosticsProtocolErrorsKeepRPCSemantics(t *testing.T) {
	cs, _ := diagnosticSession(t, false)
	for _, p := range []*mcp.CallToolParams{{Name: "create_canvas", Arguments: map[string]any{}}, {Name: "private-secret-tool", Arguments: map[string]any{}}} {
		res, err := cs.CallTool(context.Background(), p)
		require.Nil(t, res)
		var wire *jsonrpc.Error
		require.True(t, errors.As(err, &wire))
		require.Equal(t, int64(jsonrpc.CodeInvalidParams), wire.Code)
		require.NotContains(t, wire.Message, "private-secret")
		var data map[string]any
		require.NoError(t, json.Unmarshal(wire.Data, &data))
		require.NotEmpty(t, data["request_id"])
	}
}
func TestToolDiagnosticsConcurrentIDsAndSuccessShape(t *testing.T) {
	cs, _ := diagnosticSession(t, false)
	ids := make(chan string, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_snapshots", Arguments: map[string]any{}})
			if err != nil {
				t.Error(err)
				return
			}
			if res.IsError {
				t.Error("unexpected error")
				return
			}
			ids <- res.Meta[diagnostics.RequestIDMeta].(string)
			b, e := json.Marshal(res.StructuredContent)
			if e != nil {
				t.Error(e)
				return
			}
			var out map[string]any
			if e = json.Unmarshal(b, &out); e != nil {
				t.Error(e)
				return
			}
			if len(out) != 1 || out["snapshots"] == nil {
				t.Errorf("success payload changed: %s", b)
			}
		}()
	}
	wg.Wait()
	close(ids)
	seen := map[string]bool{}
	for id := range ids {
		require.False(t, seen[id])
		seen[id] = true
	}
	require.Len(t, seen, 8)
}

func TestToolDiagnosticsPreservesSharedSuccessAndNonToolResults(t *testing.T) {
	shared := &mcp.CallToolResult{Meta: mcp.Meta{"custom": "keep"}, Content: []mcp.Content{&mcp.TextContent{Text: "unchanged"}}, StructuredContent: map[string]any{"warnings": []string{"existing"}}}
	logger := mtlog.New(mtlog.WithSink(sinks.NewMemorySink()))
	handler := toolDiagnostics(logger)(func(context.Context, string, mcp.Request) (mcp.Result, error) { return shared, nil })
	result, err := handler(context.Background(), "tools/call", nil)
	require.NoError(t, err)
	out := result.(*mcp.CallToolResult)
	require.Equal(t, shared.Content, out.Content)
	require.Equal(t, shared.StructuredContent, out.StructuredContent)
	require.Equal(t, "keep", out.Meta["custom"])
	require.NotContains(t, shared.Meta, diagnostics.RequestIDMeta)
	result, err = handler(context.Background(), "tools/list", nil)
	require.NoError(t, err)
	require.Same(t, shared, result)
}

func TestRollbackRecoverySurvivesMCPBoundary(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	sink := sinks.NewMemorySink()
	logger := mtlog.New(mtlog.WithSink(sink), mtlog.WithFilter(diagnostics.RedactLogFilter{}))
	s, err := New(cfg, logger)
	require.NoError(t, err)
	for _, kind := range []string{"commit", "cancel", "timeout"} {
		cause := errors.New("private-user-path: write failure")
		if kind == "cancel" {
			cause = context.Canceled
		}
		if kind == "timeout" {
			cause = context.DeadlineExceeded
		}
		failure := &diagnostics.RollbackError{Cause: errors.Join(cause, errors.New("private-user-path: rollback failed")), Recovery: []diagnostics.RecoveryReference{{OutputIndex: 1, Directory: ".pixel-mcp-stage-recovery", BackupFile: ".original-backup", RollbackFailed: true}}}
		mcp.AddTool(s.mcp, &mcp.Tool{Name: "rollback_" + kind, Description: "Test fault boundary"}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, struct{}, error) {
			return nil, struct{}{}, failure
		})
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.mcp.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	defer cs.Close()
	for _, kind := range []string{"commit", "cancel", "timeout"} {
		result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "rollback_" + kind, Arguments: map[string]any{}})
		require.NoError(t, err)
		require.True(t, result.IsError)
		var payload struct {
			RequestID string                  `json:"request_id"`
			Error     diagnostics.PublicError `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &payload))
		require.Equal(t, "file_rollback_failed", payload.Error.Code)
		require.Len(t, payload.Error.Recovery, 1)
		require.Equal(t, ".original-backup", payload.Error.Recovery[0].BackupFile)
		require.True(t, payload.Error.Recovery[0].RollbackFailed)
		require.Equal(t, payload.RequestID, result.Meta[diagnostics.RequestIDMeta])
		meta, err := json.Marshal(result.Meta[diagnostics.ErrorMeta])
		require.NoError(t, err)
		var public diagnostics.PublicError
		require.NoError(t, json.Unmarshal(meta, &public))
		require.Equal(t, payload.Error, public)
		encoded, err := json.Marshal(result)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), "private-user-path")
		found := false
		for _, event := range sink.Events() {
			if event.Properties["RequestID"] == payload.RequestID && event.Properties["ErrorCode"] == "file_rollback_failed" {
				found = true
			}
		}
		require.True(t, found)
	}
}

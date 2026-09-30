//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func newHistoryFixture(t *testing.T, dir string, enabled bool) *behaviorFixture {
	t.Helper()
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	cfg.SnapshotDir = dir
	cfg.EnableHistory = enabled
	c := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, cfg.Timeout)
	g := aseprite.NewLuaGenerator()
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	server := mcp.NewServer(&mcp.Implementation{Name: "history-test", Version: "1"}, nil)
	RegisterCanvasTools(server, c, g, cfg, logger)
	RegisterDrawingTools(server, c, g, cfg, logger)
	RegisterQuantizationTools(server, c, g, cfg, logger)
	RegisterExportTools(server, c, g, cfg, logger)
	RegisterSnapshotTools(server, cfg, logger)
	RegisterHistoryTools(server, cfg, logger)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &behaviorFixture{t, cs, c, g}
}
func historyOperations(f *behaviorFixture, p string) []any {
	return f.call("list_operation_history", map[string]any{"sprite_path": p})["operations"].([]any)
}
func TestHistoryMCPRealEditUndoAndExclusions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	f := newHistoryFixture(t, dir, true)
	p := dryRunSprite(t, f)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Empty(t, historyOperations(f, p))
	preview := f.call("flatten_layers", map[string]any{"sprite_path": p, "dry_run": true})
	require.Equal(t, true, preview["dry_run"])
	f.call("get_sprite_info", map[string]any{"sprite_path": p})
	f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "export.png"), "format": "png", "frame_number": 1})
	require.Empty(t, historyOperations(f, p))
	applied := f.call("flatten_layers", map[string]any{"sprite_path": p})
	require.NotEmpty(t, applied["warnings"])
	operations := historyOperations(f, p)
	require.Len(t, operations, 1)
	op := operations[0].(map[string]any)
	require.Equal(t, "flatten_layers", op["tool"])
	encoded, err := json.Marshal(op)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), p)
	require.NotContains(t, string(encoded), "arguments")
	actual, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NotEqual(t, before, actual)
	// Undo remains available after restart with recording disabled.
	f = newHistoryFixture(t, dir, false)
	undone := f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]})
	require.Equal(t, true, undone["success"])
	actual, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, actual)
	info, err := inspectPreviewSprite(context.Background(), f.client, p)
	require.NoError(t, err)
	require.Equal(t, 2, info.LayerCount)
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "undo_last_operation", Arguments: map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]}})
	require.NoError(t, err)
	require.True(t, result.IsError)
}
func TestHistoryMCPDisabledFailureAndInputLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	f := newHistoryFixture(t, dir, false)
	p := dryRunSprite(t, f)
	f.call("flatten_layers", map[string]any{"sprite_path": p})
	require.Empty(t, historyOperations(f, p))
	f = newHistoryFixture(t, dir, true)
	failed, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "draw_pixels", Arguments: map[string]any{"sprite_path": p, "layer_name": "missing", "frame_number": 1, "pixels": []map[string]any{{"x": 1, "y": 1, "color": "#ff0000"}}}})
	require.NoError(t, err)
	require.True(t, failed.IsError)
	require.Empty(t, historyOperations(f, p))
	// Import reserves an additional image input before the history store lock.
	source := dryRunSprite(t, f)
	image := filepath.Join(t.TempDir(), "input.png")
	f.call("export_sprite", map[string]any{"sprite_path": source, "output_path": image, "format": "png", "frame_number": 1})
	f.call("import_image", map[string]any{"sprite_path": p, "image_path": image, "layer_name": "Imported", "frame_number": 1})
	require.Len(t, historyOperations(f, p), 1)
}
func TestHistoryMCPQuantizationAndConflict(t *testing.T) {
	f := newHistoryFixture(t, filepath.Join(t.TempDir(), "store"), true)
	p := dryRunSprite(t, f)
	out := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 3, "algorithm": "kmeans", "dither": false})
	require.Equal(t, true, out["success"])
	require.NotEmpty(t, out["warnings"])
	operations := historyOperations(f, p)
	require.Len(t, operations, 1)
	f.lua(p, `local s=app.activeSprite;s.data="external change";s:saveAs(s.filename)`)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "undo_last_operation", Arguments: map[string]any{"sprite_path": p, "expected_operation_id": operations[0].(map[string]any)["operation_id"]}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

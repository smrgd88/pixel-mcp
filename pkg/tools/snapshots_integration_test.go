//go:build integration

package tools

import (
	"context"
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

func snapshotSession(t *testing.T, dir string) *mcp.ClientSession {
	t.Helper()
	cfg := testutil.LoadTestConfig(t)
	cfg.SnapshotDir = dir
	server := mcp.NewServer(&mcp.Implementation{Name: "snapshot-test", Version: "1"}, nil)
	RegisterSnapshotTools(server, cfg, mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel)))
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return cs
}
func TestSnapshotMCPPreservesNativeSpriteAndRecoversEdit(t *testing.T) {
	f := newDryRunFixture(t)
	p := dryRunSprite(t, f)
	f.lua(p, `local s=app.activeSprite;s:newEmptyFrame();s.frames[1].duration=.23;s:newTag(1,2).name="walk";s:saveAs(s.filename)`)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	dir := filepath.Join(t.TempDir(), "snapshots")
	session := snapshotSession(t, dir)
	sf := &behaviorFixture{t, session, f.client, f.gen}
	created := sf.call("create_snapshot", map[string]any{"sprite_path": p, "label": "before edit"})
	id := created["snapshot"].(map[string]any)["snapshot_id"].(string)
	unchanged, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, unchanged)
	f.lua(p, `local s=app.activeSprite;s.data="changed metadata";s.layers[1].name="Edited";s:saveAs(s.filename)`)
	edited, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NotEqual(t, before, edited)
	// A fresh MCP server must discover and restore a previous server's copy.
	sf.session = snapshotSession(t, dir)
	listed := sf.call("list_snapshots", map[string]any{"sprite_path": p})
	require.Len(t, listed["snapshots"], 1)
	restored := sf.call("restore_snapshot", map[string]any{"sprite_path": p, "snapshot_id": id})
	backup := restored["backup_snapshot"].(map[string]any)["snapshot_id"].(string)
	actual, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, actual)
	state, err := inspectPreviewSprite(context.Background(), f.client, p)
	require.NoError(t, err)
	require.Equal(t, 2, state.FrameCount)
	require.Equal(t, 2, state.LayerCount)
	sf.call("restore_snapshot", map[string]any{"sprite_path": p, "snapshot_id": backup})
	actual, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, edited, actual)
	sf.call("delete_snapshot", map[string]any{"snapshot_id": id})
	failed, err := sf.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "restore_snapshot", Arguments: map[string]any{"sprite_path": p, "snapshot_id": id}})
	require.NoError(t, err)
	require.True(t, failed.IsError)
	actual, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, edited, actual)
}
func TestSnapshotMCPSchemaAndRejection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	cs := snapshotSession(t, dir)
	list, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, list.Tools, 4)
	for _, tool := range list.Tools {
		require.NotNil(t, tool.InputSchema)
		require.NotNil(t, tool.OutputSchema)
	}
	for _, args := range []map[string]any{{}, {"snapshot_id": "../escape"}, {"snapshot_id": "00000000-0000-0000-0000-000000000000"}} {
		result, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "delete_snapshot", Arguments: args})
		if len(args) == 0 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.True(t, result.IsError)
		}
	}
	f := newDryRunFixture(t)
	p := f.sprite(aseprite.ColorModeIndexed)
	sf := &behaviorFixture{t, cs, f.client, f.gen}
	created := sf.call("create_snapshot", map[string]any{"sprite_path": p})
	require.NotEmpty(t, created["snapshot"])
}

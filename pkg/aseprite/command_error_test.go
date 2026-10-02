package aseprite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
)

func TestCommandDiagnosticsRealLuaAndProcessErrors(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	dir := t.TempDir()
	client := NewClient(cfg.AsepritePath, dir, cfg.Timeout)
	for _, script := range []string{`error("sensitive-user-content")`, `local = broken`} {
		_, err := client.ExecuteLua(context.Background(), script, "")
		require.Error(t, err)
		require.Equal(t, "lua_error", diagnostics.Classify(err).Code)
	}
	out, err := client.ExecuteLua(context.Background(), `print("preserved output")`, "")
	require.NoError(t, err)
	require.Contains(t, out, "preserved output")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	missing := NewClient(filepath.Join(dir, "missing-executable"), dir, time.Second)
	_, err = missing.ExecuteCommand(context.Background(), []string{"--version"})
	require.Error(t, err)
	require.Equal(t, "aseprite_execution_failed", diagnostics.Classify(err).Code)
	limited := NewClient(cfg.AsepritePath, dir, 250*time.Millisecond)
	_, err = limited.ExecuteLua(context.Background(), `while true do end`, "")
	require.Error(t, err)
	require.Equal(t, "timeout", diagnostics.Classify(err).Code)
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.ExecuteCommand(ctx, []string{"--version"})
	require.Error(t, err)
	require.Equal(t, "cancelled", diagnostics.Classify(err).Code)
}

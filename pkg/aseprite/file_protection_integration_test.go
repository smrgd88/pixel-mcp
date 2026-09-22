//go:build integration

package aseprite

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIntegrationFileProtectionLuaFailure(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	c := NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	path := filepath.Join(t.TempDir(), "source.aseprite")
	_, err := c.ExecuteLua(context.Background(), NewLuaGenerator().CreateCanvas(8, 8, ColorModeRGB, path), "")
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, script := range []string{
		`app.activeSprite:resize(1,1);app.activeSprite:saveAs(app.activeSprite.filename);error("after save")`,
		`app.activeSprite:resize(1,1);app.activeSprite:saveAs(app.activeSprite.filename);while true do end`,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err = c.ExecuteLua(ctx, script, path)
		cancel()
		require.Error(t, err)
		after, e := os.ReadFile(path)
		require.NoError(t, e)
		require.Equal(t, before, after)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	// Multi-process operations share one staging file and publish only at the end.
	err = WithSpriteAccess(context.Background(), path, true, func(ctx context.Context) error {
		_, e := c.ExecuteLua(ctx, `app.activeSprite:resize(4,4);app.activeSprite:saveAs(app.activeSprite.filename)`, path)
		if e != nil {
			return e
		}
		_, e = c.ExecuteLua(ctx, `assert(app.activeSprite.width==4);error("second step failed")`, path)
		return e
	})
	require.Error(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

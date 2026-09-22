//go:build integration

package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestIntegrationFileProtectionHandlerRollback(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	c := aseprite.NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	path := filepath.Join(t.TempDir(), "source.aseprite")
	_, err := c.ExecuteLua(context.Background(), aseprite.NewLuaGenerator().CreateCanvas(8, 8, aseprite.ColorModeRGB, path), "")
	require.NoError(t, err)
	original, err := os.ReadFile(path)
	require.NoError(t, err)
	handler := wrapWithFileProtection("test_multistep", cfg.Timeout, func(ctx context.Context, _ *mcp.CallToolRequest, input GetSpriteInfoInput) (*mcp.CallToolResult, *GetSpriteInfoOutput, error) {
		_, err := c.ExecuteLua(ctx, `app.activeSprite:resize(2,2);app.activeSprite:saveAs(app.activeSprite.filename)`, input.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		// A later process sees the first step's uncommitted changes.
		_, err = c.ExecuteLua(ctx, `assert(app.activeSprite.width==2)`, input.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("result parsing failed after saved Lua steps")
	})
	_, _, err = handler(context.Background(), nil, GetSpriteInfoInput{SpritePath: path})
	require.ErrorContains(t, err, "result parsing failed")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, after)
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestIntegrationFileProtectionMCP(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	c := aseprite.NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	gen := aseprite.NewLuaGenerator()
	server := mcp.NewServer(&mcp.Implementation{Name: "protection-test", Version: "1"}, nil)
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	RegisterCanvasTools(server, c, gen, cfg, logger)
	RegisterExportTools(server, c, gen, cfg, logger)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	defer session.Close()
	dir := t.TempDir()
	path := filepath.Join(dir, "source.aseprite")
	_, err = c.ExecuteLua(context.Background(), gen.CreateCanvas(8, 8, aseprite.ColorModeRGB, path), "")
	require.NoError(t, err)
	alias := filepath.Join(dir, "alias.aseprite")
	require.NoError(t, os.Symlink(path, alias))
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := path
			if i%2 == 0 {
				p = alias
			}
			result, e := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "add_layer", Arguments: map[string]any{"sprite_path": p, "layer_name": fmt.Sprint("layer", i)}})
			if e == nil && result.IsError {
				e = fmt.Errorf("tool failed: %v", result.Content)
			}
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_info", Arguments: map[string]any{"sprite_path": path}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	var info GetSpriteInfoOutput
	require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &info))
	require.Equal(t, 5, info.LayerCount)
	for _, target := range []string{filepath.Join(dir, "nested", "copy.aseprite"), path} {
		result, err = session.CallTool(context.Background(), &mcp.CallToolParams{Name: "save_as", Arguments: map[string]any{"sprite_path": path, "output_path": target}})
		require.NoError(t, err)
		require.False(t, result.IsError, "%v", result.Content)
		var out SaveAsOutput
		require.NoError(t, json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &out))
		require.Equal(t, target, out.FilePath)
		require.FileExists(t, target)
	}

	for _, includeJSON := range []bool{true, false} {
		t.Run(fmt.Sprintf("metadata_source_alias_%v", includeJSON), func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "native.aseprite")
			_, err := c.ExecuteLua(context.Background(), gen.CreateCanvas(8, 8, aseprite.ColorModeRGB, source), "")
			require.NoError(t, err)
			before, err := os.ReadFile(source)
			require.NoError(t, err)
			texture := filepath.Join(filepath.Dir(source), "sheet.png")
			require.NoError(t, os.Symlink(source, filepath.Join(filepath.Dir(source), "sheet.json")))
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_spritesheet", Arguments: map[string]any{
				"sprite_path": source, "output_path": texture, "layout": "horizontal", "padding": 0, "include_json": includeJSON,
			}})
			require.NoError(t, err)
			after, err := os.ReadFile(source)
			require.NoError(t, err)
			require.Equal(t, sha256.Sum256(before), sha256.Sum256(after), "metadata output must not overwrite source")
			require.True(t, result.IsError, "source alias must be rejected before export")
			require.NoFileExists(t, texture)
		})
	}
	stageDirs, err := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
	require.NoError(t, err)
	require.Empty(t, stageDirs)
}

//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func newDryRunFixture(t *testing.T) *behaviorFixture {
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, cfg.Timeout)
	gen := aseprite.NewLuaGenerator()
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	server := mcp.NewServer(&mcp.Implementation{Name: "dry-run-test", Version: "1"}, nil)
	RegisterCanvasTools(server, client, gen, cfg, logger)
	RegisterQuantizationTools(server, client, gen, cfg, logger)
	RegisterAutoShadingTools(server, client, gen, cfg, logger)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &behaviorFixture{t, cs, client, gen}
}
func dryRunSprite(t *testing.T, f *behaviorFixture) string {
	t.Helper()
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB)
for y=0,15 do for x=0,15 do im:drawPixel(x,y,app.pixelColor.rgba(math.floor(x/4)*70,math.floor(y/4)*70,80,255)) end end
s.layers[1].name="Base";s:newCel(s.layers[1],1,im)
local layer=s:newLayer();layer.name="Overlay";local patch=Image(4,4,ColorMode.RGB);patch:clear(app.pixelColor.rgba(255,255,255,80));s:newCel(layer,1,patch,Point(6,6));s.data="artist metadata";s:saveAs(s.filename)`)
	return p
}
func TestDryRunPreservesSourceAndMatchesApply(t *testing.T) {
	cases := []struct {
		name, tool string
		args       map[string]any
	}{
		{"flatten", "flatten_layers", map[string]any{}},
		{"median_cut", "quantize_palette", map[string]any{"target_colors": 3, "algorithm": "median_cut", "dither": false}},
		{"kmeans", "quantize_palette", map[string]any{"target_colors": 3, "algorithm": "kmeans", "dither": true}},
		{"octree", "quantize_palette", map[string]any{"target_colors": 3, "algorithm": "octree", "dither": false, "convert_to_indexed": false}},
		{"auto_shading", "apply_auto_shading", map[string]any{"layer_name": "Base", "frame_number": 1, "light_direction": "top_left", "intensity": 0.6, "style": "cell", "hue_shift": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDryRunFixture(t)
			p := dryRunSprite(t, f)
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			info, err := os.Stat(p)
			require.NoError(t, err)
			args := tc.args
			args["sprite_path"] = p
			args["dry_run"] = true
			preview := f.call(tc.tool, args)
			require.Equal(t, true, preview["dry_run"])
			details := preview["preview"].(map[string]any)
			require.Equal(t, true, details["would_change_file"])
			require.Equal(t, float64(2), details["before"].(map[string]any)["layer_count"])
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterInfo, err := os.Stat(p)
			require.NoError(t, err)
			require.True(t, os.SameFile(info, afterInfo))
			require.Equal(t, info.Mode(), afterInfo.Mode())
			require.Equal(t, info.ModTime(), afterInfo.ModTime())
			again := f.call(tc.tool, args)
			require.Equal(t, preview, again, "same input must produce the same preview")
			args["dry_run"] = false
			applied := f.call(tc.tool, args)
			delete(preview, "dry_run")
			delete(preview, "preview")
			require.Equal(t, preview, applied, "operation output must match actual apply")
			actual, err := os.ReadFile(p)
			require.NoError(t, err)
			require.NotEqual(t, before, actual)
			encoded, err := json.Marshal(details["after"])
			require.NoError(t, err)
			var expectedState PreviewSpriteState
			require.NoError(t, json.Unmarshal(encoded, &expectedState))
			actualState, err := inspectPreviewSprite(context.Background(), f.client, p)
			require.NoError(t, err)
			require.Equal(t, expectedState, actualState)
			if tc.tool == "flatten_layers" {
				require.Equal(t, float64(1), details["after"].(map[string]any)["layer_count"])
			}
		})
	}
}
func TestDryRunUnsupportedToolDoesNotWrite(t *testing.T) {
	f := newDryRunFixture(t)
	p := dryRunSprite(t, f)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "add_layer", Arguments: map[string]any{"sprite_path": p, "layer_name": "Unsafe", "dry_run": true}})
	if err == nil {
		require.True(t, result.IsError)
	}
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestDryRunReadOnlyAndInvalidInput(t *testing.T) {
	f := newDryRunFixture(t)
	p := dryRunSprite(t, f)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NoError(t, os.Chmod(p, 0400))
	defer os.Chmod(p, 0600)
	out := f.call("flatten_layers", map[string]any{"sprite_path": p, "dry_run": true})
	require.Equal(t, true, out["dry_run"])
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	info, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0400), info.Mode().Perm())
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "quantize_palette", Arguments: map[string]any{"sprite_path": p, "dry_run": true, "target_colors": 1, "algorithm": "median_cut", "dither": false}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	after, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	// A preview is permitted on readable sources; applying still checks write permission.
	result, err = f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "flatten_layers", Arguments: map[string]any{"sprite_path": p}})
	require.NoError(t, err)
	require.True(t, result.IsError)
}
func TestDryRunSchemaAndLegacyOutput(t *testing.T) {
	f := newDryRunFixture(t)
	list, err := f.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	supported := map[string]bool{"flatten_layers": true, "quantize_palette": true, "apply_auto_shading": true}
	for _, tool := range list.Tools {
		var in, out struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		data, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &in))
		data, err = json.Marshal(tool.OutputSchema)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(data, &out))
		if supported[tool.Name] {
			require.Contains(t, in.Properties, "dry_run")
			require.NotContains(t, in.Required, "dry_run")
			require.Contains(t, out.Properties, "dry_run")
			require.Contains(t, out.Properties, "preview")
			require.NotContains(t, out.Required, "dry_run")
			require.NotContains(t, out.Required, "preview")
		} else {
			require.NotContains(t, in.Properties, "dry_run")
		}
	}
	p := dryRunSprite(t, f)
	result := f.call("flatten_layers", map[string]any{"sprite_path": p})
	require.NotContains(t, result, "dry_run")
	require.NotContains(t, result, "preview")
	require.Contains(t, result, "warnings")
}

func TestDryRunErrorAfterCopyMutationDoesNotPublish(t *testing.T) {
	f := newDryRunFixture(t)
	p := dryRunSprite(t, f)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	handler := maybeWrapWithDryRun("flatten_layers", f.client, logger, false, 5*time.Second, func(ctx context.Context, _ *mcp.CallToolRequest, input FlattenLayersInput) (*mcp.CallToolResult, *FlattenLayersOutput, error) {
		_, err := f.client.ExecuteLua(ctx, f.gen.FlattenLayers(), input.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, fmt.Errorf("failure after temporary edit")
	})
	_, out, err := handler(context.Background(), nil, FlattenLayersInput{SpritePath: p, DryRun: true})
	require.ErrorContains(t, err, "failure after temporary edit")
	require.Nil(t, out)
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

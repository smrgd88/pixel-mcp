//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestOperationWarningsViaMCP(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, cfg.Timeout)
	gen := aseprite.NewLuaGenerator()
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	server := mcp.NewServer(&mcp.Implementation{Name: "warnings-test", Version: "1"}, nil)
	RegisterCanvasTools(server, client, gen, cfg, logger)
	RegisterTransformTools(server, client, gen, cfg, logger)
	RegisterQuantizationTools(server, client, gen, cfg, logger)
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	defer ss.Close()
	mc := mcp.NewClient(&mcp.Implementation{Name: "warnings-client", Version: "1"}, nil)
	session, err := mc.Connect(ctx, ct, nil)
	require.NoError(t, err)
	defer session.Close()

	// Check the advertised wire schema as well as successful tool responses.
	list, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	checked := 0
	for _, tool := range list.Tools {
		if tool.Name != "flatten_layers" && tool.Name != "quantize_palette" && tool.Name != "scale_sprite" {
			continue
		}
		encoded, err := json.Marshal(tool.OutputSchema)
		require.NoError(t, err)
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
			Required   []string                   `json:"required"`
		}
		require.NoError(t, json.Unmarshal(encoded, &schema))
		require.Contains(t, schema.Properties, "warnings")
		require.NotContains(t, schema.Required, "warnings")
		checked++
	}
	require.Equal(t, 3, checked)

	for _, tt := range []struct {
		name, tool string
		args       map[string]any
		codes      []string
	}{
		{"flatten", "flatten_layers", map[string]any{}, []string{"layer_flattening"}},
		{"quantize default conversion", "quantize_palette", map[string]any{"target_colors": 2, "algorithm": "", "dither": false}, []string{"palette_quantization", "color_mode_conversion"}},
		{"quantize keep mode", "quantize_palette", map[string]any{"target_colors": 2, "convert_to_indexed": false, "algorithm": "median_cut", "dither": false}, []string{"palette_quantization"}},
		{"scale default", "scale_sprite", map[string]any{"scale_x": 2, "scale_y": 2, "algorithm": ""}, nil},
		{"scale nearest", "scale_sprite", map[string]any{"scale_x": 2, "scale_y": 2, "algorithm": "nearest"}, nil},
		{"scale bilinear", "scale_sprite", map[string]any{"scale_x": 2, "scale_y": 2, "algorithm": "bilinear"}, []string{"resampling"}},
		{"scale rotsprite", "scale_sprite", map[string]any{"scale_x": 2, "scale_y": 2, "algorithm": "rotsprite"}, []string{"resampling"}},
		{"scale identity", "scale_sprite", map[string]any{"scale_x": 1, "scale_y": 1, "algorithm": "bilinear"}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := testutil.TempSpritePath(t, "warnings.aseprite")
			_, err := client.ExecuteLua(ctx, gen.CreateCanvas(8, 8, aseprite.ColorModeRGB, path), "")
			require.NoError(t, err)
			_, err = client.ExecuteLua(ctx, gen.DrawRectangle("Layer 1", 1, 0, 0, 8, 8, aseprite.Color{R: 255, A: 255}, true, false), path)
			require.NoError(t, err)
			tt.args["sprite_path"] = path
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			require.NoError(t, err)
			require.False(t, result.IsError, "%+v", result.Content)
			require.NotNil(t, result.StructuredContent)
			structured, err := json.Marshal(result.StructuredContent)
			require.NoError(t, err)
			var response struct {
				Success  bool          `json:"success"`
				Warnings []ToolWarning `json:"warnings"`
			}
			require.NoError(t, json.Unmarshal(structured, &response))
			require.True(t, response.Success)
			var codes []string
			for _, warning := range response.Warnings {
				codes = append(codes, warning.Code)
				require.NotEmpty(t, warning.Message)
			}
			require.Equal(t, tt.codes, codes)
			if len(tt.codes) == 0 {
				require.NotContains(t, string(structured), `"warnings"`)
			}
			require.NotEmpty(t, result.Content)
			text, ok := result.Content[0].(*mcp.TextContent)
			require.True(t, ok)
			require.JSONEq(t, string(structured), text.Text)
			var legacy struct {
				Success bool `json:"success"`
			}
			require.NoError(t, json.Unmarshal([]byte(text.Text), &legacy))
			require.True(t, legacy.Success)
		})
	}

	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{"flatten_layers", map[string]any{}},
		{"quantize_palette", map[string]any{"target_colors": 2, "algorithm": "", "dither": false}},
		{"scale_sprite", map[string]any{"scale_x": 2, "scale_y": 2, "algorithm": "bilinear"}},
	} {
		t.Run(tt.tool+" failure", func(t *testing.T) {
			tt.args["sprite_path"] = testutil.TempSpritePath(t, "missing.aseprite")
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Nil(t, result.StructuredContent)
			require.NotEmpty(t, result.Content)
			require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "sprite file not found")
		})
	}
}

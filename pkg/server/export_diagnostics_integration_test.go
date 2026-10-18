//go:build integration

package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
)

func TestExportOptionsDiagnostics(t *testing.T) {
	cs, _ := diagnosticSession(t, true)
	created, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_canvas", Arguments: map[string]any{"width": 8, "height": 8, "color_mode": "rgb"}})
	require.NoError(t, err)
	require.False(t, created.IsError)
	var canvas struct {
		Path string `json:"file_path"`
	}
	raw, err := json.Marshal(created.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &canvas))
	dest := filepath.Join(t.TempDir(), "sheet.png")
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_spritesheet", Arguments: map[string]any{"sprite_path": canvas.Path, "output_path": dest, "layout": "horizontal", "padding": 0, "include_json": true}})
	require.NoError(t, err)
	require.False(t, r.IsError)
	require.NotEmpty(t, r.Meta[diagnostics.RequestIDMeta])
	var success map[string]any
	raw, err = json.Marshal(r.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &success))
	require.Len(t, success, 3)
	require.Equal(t, float64(1), success["frame_count"])
	require.Equal(t, dest, success["spritesheet_path"])
	for _, tc := range []struct {
		tool  string
		extra map[string]any
		code  string
	}{
		{"export_sprite", map[string]any{"frame_number": 2}, "invalid_arguments"},
		{"export_sprite", map[string]any{"frame_start": 2}, "lua_error"},
		{"export_sprite", map[string]any{"tag": "secret-missing-tag"}, "lua_error"},
		{"export_spritesheet", map[string]any{"overwrite": false}, "invalid_arguments"},
		{"export_spritesheet", map[string]any{"layer_id": "1", "expected_revision": strings.Repeat("0", 64)}, "file_changed"},
	} {
		args := map[string]any{"sprite_path": canvas.Path, "output_path": dest}
		if tc.tool == "export_sprite" {
			args["format"] = "png"
			args["frame_number"] = 0
		} else {
			args["layout"] = "horizontal"
			args["padding"] = 0
			args["include_json"] = false
		}
		for k, v := range tc.extra {
			args[k] = v
		}
		out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: args})
		require.NoError(t, err)
		require.True(t, out.IsError)
		var payload struct {
			Error diagnostics.PublicError `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &payload))
		require.Equal(t, tc.code, payload.Error.Code)
		data, err := json.Marshal(out)
		require.NoError(t, err)
		require.NotContains(t, string(data), canvas.Path)
		require.NotContains(t, string(data), "secret-missing-tag")
		require.NotEmpty(t, out.Meta[diagnostics.RequestIDMeta])
	}
}

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
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestToolDiagnosticsRealSuccessWarningsAndLuaError(t *testing.T) {
	cs, _ := diagnosticSession(t, true)
	create, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_canvas", Arguments: map[string]any{"width": 8, "height": 8, "color_mode": "rgb"}})
	require.NoError(t, err)
	require.False(t, create.IsError)
	b, err := json.Marshal(create.StructuredContent)
	require.NoError(t, err)
	var canvas struct {
		Path string `json:"file_path"`
	}
	require.NoError(t, json.Unmarshal(b, &canvas))
	require.NotEmpty(t, canvas.Path)
	structure, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_structure", Arguments: map[string]any{"sprite_path": canvas.Path}})
	require.NoError(t, err)
	require.False(t, structure.IsError)
	require.NotEmpty(t, structure.Meta[diagnostics.RequestIDMeta])

	var structureData struct {
		Revision string `json:"revision"`
	}
	rawStructure, err := json.Marshal(structure.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(rawStructure, &structureData))
	require.Len(t, structureData.Revision, 64)
	id := create.Meta[diagnostics.RequestIDMeta]
	require.NotEmpty(t, id)
	for _, tc := range []struct {
		name string
		args map[string]any
		code string
	}{
		{"unlink_cel", map[string]any{"sprite_path": canvas.Path, "layer_id": "1", "frame_number": 1, "expected_revision": strings.Repeat("0", 64)}, "file_changed"},
		{"unlink_cel", map[string]any{"sprite_path": canvas.Path, "layer_id": "1", "frame_number": 0, "expected_revision": structureData.Revision}, "invalid_arguments"},
		{"unlink_cel", map[string]any{"sprite_path": canvas.Path, "layer_id": "99", "frame_number": 1, "expected_revision": structureData.Revision}, "lua_error"},
		{"unlink_cel", map[string]any{"sprite_path": filepath.Join(t.TempDir(), "sensitive-name.aseprite"), "layer_id": "1", "frame_number": 1, "expected_revision": structureData.Revision}, "not_found"},
		{"set_cel_properties", map[string]any{"sprite_path": canvas.Path, "layer_id": "1", "frame_number": 1, "expected_revision": strings.Repeat("0", 64), "x": 1}, "file_changed"},
		{"set_cel_properties", map[string]any{"sprite_path": canvas.Path, "layer_id": "1", "frame_number": 1, "expected_revision": structureData.Revision, "opacity": 256}, "invalid_arguments"},
		{"set_cel_properties", map[string]any{"sprite_path": canvas.Path, "layer_id": "99", "frame_number": 1, "expected_revision": structureData.Revision, "x": 1}, "lua_error"},
		{"get_sprite_structure", map[string]any{"sprite_path": canvas.Path, "page_size": 101}, "invalid_arguments"},
		{"get_sprite_structure", map[string]any{"sprite_path": canvas.Path, "layer_id": "99"}, "lua_error"},
		{"get_sprite_structure", map[string]any{"sprite_path": filepath.Join(t.TempDir(), "sensitive-name.aseprite")}, "not_found"},
		{"delete_layer", map[string]any{"sprite_path": canvas.Path, "layer_name": "private-user-layer"}, "lua_error"},
		{"get_sprite_info", map[string]any{"sprite_path": filepath.Join(t.TempDir(), "sensitive-name.aseprite")}, "not_found"},
		{"delete_snapshot", map[string]any{"snapshot_id": "invalid-id"}, "snapshot_invalid"},
	} {
		r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
		require.NoError(t, err)
		require.True(t, r.IsError)
		var payload struct {
			Error diagnostics.PublicError `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &payload))
		require.Equal(t, tc.code, payload.Error.Code)
		encoded, err := json.Marshal(r)
		require.NoError(t, err)
		require.NotContains(t, string(encoded), canvas.Path)
		require.NotContains(t, string(encoded), "private-user-layer")
		require.NotContains(t, string(encoded), "sensitive-name")
	}
	flatten, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "flatten_layers", Arguments: map[string]any{"sprite_path": canvas.Path, "dry_run": true}})
	require.NoError(t, err)
	require.False(t, flatten.IsError)
	b, err = json.Marshal(flatten.StructuredContent)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(b, &payload))
	require.Equal(t, true, payload["dry_run"])
	require.NotEmpty(t, payload["warnings"])
	require.NotContains(t, payload, "request_id")
	require.NotEqual(t, id, flatten.Meta[diagnostics.RequestIDMeta])
}

func TestCelPropertiesDiagnosticsSuccess(t *testing.T) {
	cs, _ := diagnosticSession(t, true)
	create, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_canvas", Arguments: map[string]any{"width": 8, "height": 8, "color_mode": "rgb"}})
	require.NoError(t, err)
	require.False(t, create.IsError)
	var canvas struct {
		Path string `json:"file_path"`
	}
	b, err := json.Marshal(create.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &canvas))
	draw, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "draw_pixels", Arguments: map[string]any{"sprite_path": canvas.Path, "layer_name": "Layer 1", "frame_number": 1, "pixels": []map[string]any{{"x": 0, "y": 0, "color": "#FFFFFFFF"}}}})
	require.NoError(t, err)
	require.False(t, draw.IsError)
	s, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_structure", Arguments: map[string]any{"sprite_path": canvas.Path}})
	require.NoError(t, err)
	require.False(t, s.IsError)
	var state struct {
		Revision string `json:"revision"`
	}
	b, err = json.Marshal(s.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &state))
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_cel_properties", Arguments: map[string]any{"sprite_path": canvas.Path, "layer_id": "1", "frame_number": 1, "expected_revision": state.Revision, "opacity": 0}})
	require.NoError(t, err)
	require.False(t, r.IsError)
	require.NotEmpty(t, r.Meta[diagnostics.RequestIDMeta])
	require.NotEqual(t, s.Meta[diagnostics.RequestIDMeta], r.Meta[diagnostics.RequestIDMeta])
	b, err = json.Marshal(r.StructuredContent)
	require.NoError(t, err)
	require.NotContains(t, string(b), canvas.Path)
	require.NotContains(t, string(b), "request_id")
	require.Contains(t, string(b), `"opacity":0`)
}

func TestUnlinkCelDiagnosticsSuccess(t *testing.T) {
	cs, _ := diagnosticSession(t, true)
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	p := filepath.Join(t.TempDir(), "private-unlink.aseprite")
	_, err := client.ExecuteLua(context.Background(), `local s=Sprite(8,8);local l=s.layers[1];l:cel(1).image:drawPixel(0,0,0xffffffff)
s:newEmptyFrame();app.range.layers={l};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
s:saveAs("`+aseprite.EscapeString(p)+`")`, "")
	require.NoError(t, err)
	s, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_structure", Arguments: map[string]any{"sprite_path": p}})
	require.NoError(t, err)
	require.False(t, s.IsError)
	var state struct {
		Revision string `json:"revision"`
	}
	b, err := json.Marshal(s.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &state))
	r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "unlink_cel", Arguments: map[string]any{"sprite_path": p, "layer_id": "1", "frame_number": 1, "expected_revision": state.Revision}})
	require.NoError(t, err)
	require.False(t, r.IsError)
	require.NotEmpty(t, r.Meta[diagnostics.RequestIDMeta])
	require.NotEqual(t, s.Meta[diagnostics.RequestIDMeta], r.Meta[diagnostics.RequestIDMeta])
	b, err = json.Marshal(r.StructuredContent)
	require.NoError(t, err)
	require.NotContains(t, string(b), p)
	require.NotContains(t, string(b), "request_id")
	require.Contains(t, string(b), `"remaining_linked_cels":[{"frame_number":2,"layer_id":"1"}]`)
	list, err := cs.ListTools(context.Background(), nil)
	require.NoError(t, err)
	for _, tool := range list.Tools {
		if tool.Name == "unlink_cel" {
			b, err = json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			var schema struct {
				Required []string `json:"required"`
			}
			require.NoError(t, json.Unmarshal(b, &schema))
			require.ElementsMatch(t, []string{"sprite_path", "layer_id", "frame_number", "expected_revision"}, schema.Required)
		}
	}
}

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

func TestLayerEditDiagnostics(t *testing.T) {
	cs, _ := diagnosticSession(t, true)
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		r, e := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, e)
		return r
	}
	create := call("create_canvas", map[string]any{"width": 8, "height": 8, "color_mode": "rgb"})
	require.False(t, create.IsError)
	var canvas struct {
		Path string `json:"file_path"`
	}
	b, e := json.Marshal(create.StructuredContent)
	require.NoError(t, e)
	require.NoError(t, json.Unmarshal(b, &canvas))
	revision := func() string {
		t.Helper()
		r := call("get_sprite_structure", map[string]any{"sprite_path": canvas.Path})
		require.False(t, r.IsError)
		var s struct {
			Revision string `json:"revision"`
		}
		b, e := json.Marshal(r.StructuredContent)
		require.NoError(t, e)
		require.NoError(t, json.Unmarshal(b, &s))
		return s.Revision
	}
	for _, tool := range []string{"set_layer_properties", "move_layer", "create_layer_group"} {
		for _, tc := range []struct {
			code  string
			patch map[string]any
		}{
			{"file_changed", map[string]any{"expected_revision": strings.Repeat("0", 64)}},
			{"invalid_arguments", map[string]any{"expected_revision": "invalid"}},
			{"lua_error", map[string]any{"layer_id": "9", "parent_id": "9"}},
			{"not_found", map[string]any{"sprite_path": filepath.Join(t.TempDir(), "sensitive.aseprite")}},
		} {
			args := map[string]any{"sprite_path": canvas.Path, "expected_revision": revision(), "layer_id": "1", "parent_id": "", "stack_index": 1, "name": "private-user-layer"}
			for k, v := range tc.patch {
				args[k] = v
			}
			if tool == "set_layer_properties" {
				delete(args, "parent_id")
				delete(args, "stack_index")
			}
			if tool == "move_layer" {
				delete(args, "name")
			}
			if tool == "create_layer_group" {
				delete(args, "layer_id")
			}
			r := call(tool, args)
			require.True(t, r.IsError)
			var payload struct {
				Error diagnostics.PublicError `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &payload))
			require.Equal(t, tc.code, payload.Error.Code)
			require.NotEmpty(t, r.Meta[diagnostics.RequestIDMeta])
			b, e := json.Marshal(r)
			require.NoError(t, e)
			for _, s := range []string{canvas.Path, "sensitive", "private-user-layer"} {
				require.NotContains(t, string(b), s)
			}
		}
	}
	for _, tc := range []struct {
		tool  string
		patch map[string]any
	}{
		{"create_layer_group", map[string]any{"parent_id": "", "name": "g", "stack_index": 2}},
		{"move_layer", map[string]any{"layer_id": "1", "parent_id": "2", "stack_index": 1}},
		{"set_layer_properties", map[string]any{"layer_id": "1/1", "opacity": 0}},
	} {
		args := map[string]any{"sprite_path": canvas.Path, "expected_revision": revision()}
		for k, v := range tc.patch {
			args[k] = v
		}
		r := call(tc.tool, args)
		require.False(t, r.IsError)
		require.NotEmpty(t, r.Meta[diagnostics.RequestIDMeta])
		b, e := json.Marshal(r.StructuredContent)
		require.NoError(t, e)
		require.NotContains(t, string(b), canvas.Path)
		require.NotContains(t, string(b), "request_id")
	}
}

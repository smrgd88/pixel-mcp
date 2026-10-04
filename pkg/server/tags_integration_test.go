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

func TestTagsDiagnostics(t *testing.T) {
	cs, _ := diagnosticSession(t, true)
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		r, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		require.NotEmpty(t, r.Meta[diagnostics.RequestIDMeta])
		return r
	}
	create := call("create_canvas", map[string]any{"width": 8, "height": 8, "color_mode": "rgb"})
	require.False(t, create.IsError)
	var canvas struct {
		Path string `json:"file_path"`
	}
	b, err := json.Marshal(create.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &canvas))
	r := call("create_tag", map[string]any{"sprite_path": canvas.Path, "tag_name": "tag", "from_frame": 1, "to_frame": 1, "direction": "forward"})
	require.False(t, r.IsError)
	read := call("get_sprite_tags", map[string]any{"sprite_path": canvas.Path})
	require.False(t, read.IsError)
	var state struct {
		Revision string `json:"revision"`
	}
	b, err = json.Marshal(read.StructuredContent)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(b, &state))
	for _, tc := range []struct {
		name string
		args map[string]any
		code string
	}{
		{"get_sprite_tags", map[string]any{"sprite_path": ""}, "invalid_arguments"},
		{"get_sprite_tags", map[string]any{"sprite_path": filepath.Join(t.TempDir(), "private-tag.aseprite")}, "not_found"},
		{"set_tag_properties", map[string]any{"sprite_path": canvas.Path, "tag_id": 1, "expected_revision": strings.Repeat("0", 64), "repeats": 1}, "file_changed"},
		{"set_tag_properties", map[string]any{"sprite_path": canvas.Path, "tag_id": 1, "expected_revision": state.Revision, "repeats": -1}, "invalid_arguments"},
		{"set_tag_properties", map[string]any{"sprite_path": canvas.Path, "tag_id": 2, "expected_revision": state.Revision, "repeats": 1}, "lua_error"},
		{"set_tag_properties", map[string]any{"sprite_path": filepath.Join(t.TempDir(), "private-tag.aseprite"), "tag_id": 1, "expected_revision": state.Revision, "repeats": 1}, "not_found"},
	} {
		r := call(tc.name, tc.args)
		require.True(t, r.IsError)
		var payload struct {
			Error diagnostics.PublicError `json:"error"`
		}
		require.NoError(t, json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &payload))
		require.Equal(t, tc.code, payload.Error.Code)
		b, err = json.Marshal(r)
		require.NoError(t, err)
		require.NotContains(t, string(b), canvas.Path)
		require.NotContains(t, string(b), "private-tag")
	}
	r = call("set_tag_properties", map[string]any{"sprite_path": canvas.Path, "tag_id": 1, "expected_revision": state.Revision, "repeats": 1})
	require.False(t, r.IsError)
	require.NotEqual(t, read.Meta[diagnostics.RequestIDMeta], r.Meta[diagnostics.RequestIDMeta])
	b, err = json.Marshal(r.StructuredContent)
	require.NoError(t, err)
	require.Contains(t, string(b), `"repeats":1`)
	require.NotContains(t, string(b), "request_id")
}

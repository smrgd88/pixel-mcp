//go:build integration

package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
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
	id := create.Meta[diagnostics.RequestIDMeta]
	require.NotEmpty(t, id)
	for _, tc := range []struct {
		name string
		args map[string]any
		code string
	}{
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

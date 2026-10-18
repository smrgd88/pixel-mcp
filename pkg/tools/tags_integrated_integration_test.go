//go:build integration

package tools

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Exercise the merged registrations and one shared revision/history chain across
// group creation, layer movement/properties, unlink and tag editing.
func TestTagsIntegratedRevisionAndUndo(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, layerFixture)
	original, err := os.ReadFile(p)
	require.NoError(t, err)
	tags := readTags(t, f, p)
	structure := readStructure(t, f, map[string]any{"sprite_path": p})
	require.Equal(t, structure.Revision, tags.Revision)

	group := editLayer(t, f, p, "create_layer_group", map[string]any{"name": "same", "parent_id": "", "stack_index": 3})
	require.Equal(t, "3", group["layer_id"])
	moved := editLayer(t, f, p, "move_layer", map[string]any{"layer_id": "2/1/1", "parent_id": "3", "stack_index": 1})
	require.Equal(t, "3/1", moved["layer_id"])
	rejectTag(t, f, p, map[string]any{"sprite_path": p, "tag_id": 1, "expected_revision": tags.Revision, "repeats": 0}, "stale sprite revision")
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "3/1", "name": "", "visible": false, "opacity": 0})

	state := readStructure(t, f, map[string]any{"sprite_path": p})
	unlinked := f.call("unlink_cel", map[string]any{"sprite_path": p, "layer_id": "3/1", "frame_number": 2, "expected_revision": state.Revision})
	tags = readTags(t, f, p)
	require.Equal(t, unlinked["revision"], tags.Revision)
	beforeTag, err := os.ReadFile(p)
	require.NoError(t, err)
	edited := f.call("set_tag_properties", map[string]any{"sprite_path": p, "tag_id": 1, "expected_revision": tags.Revision, "direction": "pingpong_reverse", "repeats": 0})
	state = readStructure(t, f, map[string]any{"sprite_path": p})
	require.Equal(t, edited["revision"], state.Revision)
	rejectLayer(t, f, p, "set_layer_properties", map[string]any{"sprite_path": p, "layer_id": "3/1", "expected_revision": tags.Revision, "name": "changed"}, "stale sprite revision")
	f.lua(p, `local s=app.activeSprite;local l=s.layers[3].layers[1]
assert(l.name=="" and not l.isVisible and l.opacity==0)
assert(l:cel(1).image~=l:cel(2).image and l:cel(1).image.bytes==l:cel(2).image.bytes)
assert(l:cel(1).position==Point(1,2) and l:cel(2).position==Point(1,2))
assert(l:cel(2).properties("artist").text=="cel property")
assert(s.tags[1].name=="keep" and s.tags[1].data=="tag data" and s.tags[1].repeats==0 and s.tags[1].aniDir==AniDir.PING_PONG_REVERSE)
assert(s.tags[1].fromFrame.frameNumber==1 and s.tags[1].toFrame.frameNumber==2)
assert(math.abs(s.frames[2].duration-0.25)<0.001 and s.properties("artist").text=="sprite property")`)

	ops := historyOperations(f, p)
	require.Len(t, ops, 5)
	for i, name := range []string{"set_tag_properties", "unlink_cel", "set_layer_properties", "move_layer", "create_layer_group"} {
		op := ops[i].(map[string]any)
		require.Equal(t, name, op["tool"])
		f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]})
		if i == 0 {
			actual, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, beforeTag, actual)
		}
	}
	restored, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, original, restored)
	f.lua(p, layerPreserved)
}

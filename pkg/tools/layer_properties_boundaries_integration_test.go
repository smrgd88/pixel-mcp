//go:build integration

package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestLayerPropertiesNameBoundariesAndMixedNull(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{"ascii_limit", strings.Repeat("x", 65535)},
		{"utf8_limit", strings.Repeat("가", 21845)},
		{"escaped_unicode", "emoji 😀\b\f\r\t\u2028 \\\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;local l=s.layers[1]
l.name="original";l.opacity=173;l.data="keep metadata";s:saveAs(s.filename)`)
			row := editLayer(t, f, p, "set_layer_properties", map[string]any{
				"layer_id": "1", "name": tc.value, "opacity": nil, "visible": false,
			})
			require.Equal(t, tc.value, row["name"])
			require.Equal(t, float64(173), row["opacity"])
			require.Equal(t, false, row["visible"])
			// A separate batch reopens the published file: null keeps the old value,
			// whereas false must be applied, even alongside a long/escaped name.
			saved := readStructure(t, f, map[string]any{"sprite_path": p})
			require.Len(t, saved.Layers, 1)
			require.Equal(t, tc.value, saved.Layers[0].Name)
			require.NotNil(t, saved.Layers[0].Opacity)
			require.Equal(t, 173, *saved.Layers[0].Opacity)
			require.False(t, saved.Layers[0].Visible)
			require.True(t, saved.Layers[0].Editable)
			f.lua(p, `assert(app.activeSprite.layers[1].data=="keep metadata")`)
		})
	}
	for _, tc := range []struct{ name, value string }{
		{"ascii_over_limit", strings.Repeat("x", 65536)},
		// Fewer than 65535 runes, but more than 65535 UTF-8 bytes.
		{"utf8_over_limit", strings.Repeat("가", 21846)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			rejectLayer(t, f, p, "set_layer_properties", layerArgs(t, f, p, map[string]any{
				"layer_id": "1", "name": tc.value,
			}), "name must fit native string")
		})
	}
}

func TestLayerMoveAboveBackground(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(1,1);im:clear(0xffffffff)
s:newCel(s.layers[1],1,im);app.activeLayer=s.layers[1];app.command.BackgroundFromLayer()
assert(s.layers[1].isBackground)
local g=s:newGroup();g.name="group";g.data="keep group"
local a=s:newLayer();a.parent=g;a.data="moving";s:newCel(a,1,Image(im),Point(2,3))
s:saveAs(s.filename)`)
	rejectLayer(t, f, p, "move_layer", layerArgs(t, f, p, map[string]any{
		"layer_id": "2/1", "parent_id": "", "stack_index": 1,
	}), "Cannot move below background")
	row := editLayer(t, f, p, "move_layer", map[string]any{
		"layer_id": "2/1", "parent_id": "", "stack_index": 2,
	})
	require.Equal(t, "2", row["layer_id"])
	require.Equal(t, "", row["parent_id"])
	f.lua(p, `local s=app.activeSprite
assert(#s.layers==3 and #s.frames==1 and s.layers[1].isBackground)
assert(s.layers[2].data=="moving" and s.layers[2]:cel(1).position==Point(2,3))
assert(s.layers[2]:cel(1).image:getPixel(0,0)==0xffffffff)
assert(s.layers[3].isGroup and #s.layers[3].layers==0 and s.layers[3].data=="keep group")`)
}

func TestLayerMoveRejectsTilemapDescendant(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local g=s:newGroup();g.name="group"
local nested=s:newGroup();nested.parent=g
app.command.NewLayer{name="tiles",tilemap=true,gridBounds=Rectangle(0,0,8,8),ask=false}
assert(app.activeLayer.isTilemap);app.activeLayer.parent=nested;s:saveAs(s.filename)`)
	saved := readStructure(t, f, map[string]any{"sprite_path": p})
	require.Len(t, saved.Layers, 4)
	require.Equal(t, "2", saved.Layers[1].LayerID)
	require.Equal(t, "group", saved.Layers[1].Kind)
	require.Equal(t, "2/1/1", saved.Layers[3].LayerID)
	require.Equal(t, "tilemap", saved.Layers[3].Kind)
	// The target itself is an ordinary group. Only inspection of its descendants
	// can reject this otherwise valid root reorder.
	rejectLayer(t, f, p, "move_layer", layerArgs(t, f, p, map[string]any{
		"layer_id": "2", "parent_id": "", "stack_index": 1,
	}), "Only ordinary raster layers and groups are supported")
}

func TestLayerGroupsOnlyDocument(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local original=s.layers[1]
local g=s:newGroup();g.name="only";g.data="original group";s:deleteLayer(original)
assert(#s.layers==1 and #g.layers==0);s:saveAs(s.filename)`)
	row := editLayer(t, f, p, "create_layer_group", map[string]any{
		"name": "nested", "parent_id": "1", "stack_index": 1,
	})
	require.Equal(t, "1/1", row["layer_id"])
	f.lua(p, `local s=app.activeSprite
assert(#s.layers==1 and s.layers[1].data=="original group" and #s.layers[1].layers==1)
assert(s.layers[1].layers[1].isGroup and s.layers[1].layers[1].name=="nested")`)
	row = editLayer(t, f, p, "move_layer", map[string]any{
		"layer_id": "1/1", "parent_id": "", "stack_index": 1,
	})
	require.Equal(t, "1", row["layer_id"])
	require.Equal(t, "", row["parent_id"])
	f.lua(p, `local s=app.activeSprite
assert(#s.layers==2 and #s.frames==1 and s.layers[1].name=="nested" and s.layers[2].data=="original group")
for _,l in ipairs(s.layers) do assert(l.isGroup and #l.layers==0 and #l.cels==0) end`)
}

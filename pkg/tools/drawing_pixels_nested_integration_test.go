//go:build integration

package tools

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestDrawPixelsLayerPolicy(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, state := range []string{"normal", "hidden", "locked", "hidden-locked-parent", "no-cel", "literal-name"} {
			t.Run(fmt.Sprintf("nested=%t/%s", nested, state), func(t *testing.T) {
				f := newExportBehavior(t)
				p := f.sprite(aseprite.ColorModeRGB)
				name := "paint"
				if state == "literal-name" {
					name = "group/paint\"\\\n;error('injected')--"
				}
				setup := fmt.Sprintf(`local s=app.activeSprite;local l=s.layers[1];l.name="%s"
local g=s:newGroup();g.name="outer"
local inner=s:newGroup();inner.name="inner";inner.parent=g
if %t then l.parent=inner end
if "%s"=="hidden" then l.isVisible=false end
if "%s"=="locked" then l.isEditable=false end
if "%s"=="hidden-locked-parent" then g.isVisible=false;g.isEditable=false end
if "%s"=="no-cel" then for _,c in ipairs(l.cels) do s:deleteCel(c) end end
s:newEmptyFrame();s:saveAs(s.filename)`, aseprite.EscapeString(name), nested, state, state, state, state)
				f.lua(p, setup)
				frame := 1
				if state == "no-cel" {
					frame = 2
				}
				out := f.call("draw_pixels", map[string]any{"sprite_path": p, "layer_name": name, "frame_number": frame, "pixels": []map[string]any{{"x": 1, "y": 2, "color": "#FFFFFFFF"}}})
				require.Equal(t, float64(1), out["pixels_drawn"])
				f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local l
local function find(layers) for _,v in ipairs(layers) do if v.name=="%s" then l=v end;if v.isGroup then find(v.layers) end end end
find(s.layers);assert(l and l:cel(%d))
local c=l:cel(%d);assert(c.image:getPixel(1-c.position.x,2-c.position.y)==app.pixelColor.rgba(255,255,255,255))
assert(l.isVisible==%t and l.isEditable==%t)
if %t then assert(l.parent.name=="inner" and l.parent.parent.name=="outer") end
for _,g in ipairs(s.layers) do if g.name=="outer" then assert(g.isVisible==%t and g.isEditable==%t) end end`, aseprite.EscapeString(name), frame, frame, state != "hidden", state != "locked", nested, state != "hidden-locked-parent", state != "hidden-locked-parent"))
			})
		}
	}
}

func TestDrawPixelsLayerRejectionPreservesSource(t *testing.T) {
	for _, tc := range []struct {
		name, setup, target, message string
		frame, x, y                  int
	}{
		{"duplicate-root", `local other=s:newLayer();other.name="paint"`, "paint", "Ambiguous layer name", 1, 1, 2},
		{"duplicate-nested", `local g=s:newGroup();local other=s:newLayer();other.name="paint";other.parent=g`, "paint", "Ambiguous layer name", 1, 1, 2},
		{"duplicate-siblings", `local g=s:newGroup();l.parent=g;local other=s:newLayer();other.name="paint";other.parent=g`, "paint", "Ambiguous layer name", 1, 1, 2},
		{"group-leaf-collision", `local g=s:newGroup();g.name="paint"`, "paint", "Ambiguous layer name", 1, 1, 2},
		{"missing", "", "missing", "Layer not found: missing", 1, 1, 2},
		{"group", `local g=s:newGroup();g.name="group"`, "group", "Target layer must be a raster layer", 1, 1, 2},
		{"nested-group", `local g=s:newGroup();local inner=s:newGroup();inner.name="group";inner.parent=g`, "group", "Target layer must be a raster layer", 1, 1, 2},
		{"tilemap", `app.command.NewLayer{name="tiles",tilemap=true,gridBounds=Rectangle(0,0,8,8),ask=false}`, "tiles", "Target layer must be a raster layer", 1, 1, 2},
		{"frame-zero", "", "paint", "frame_number must be at least 1", 0, 1, 2},
		{"frame-missing", `local g=s:newGroup();l.parent=g`, "paint", "Frame not found: 2", 2, 1, 2},
		{"negative-x", `local g=s:newGroup();l.parent=g`, "paint", "Pixel coordinates must be within sprite bounds", 1, -1, 2},
		{"outside-y", `local g=s:newGroup();l.parent=g`, "paint", "Pixel coordinates must be within sprite bounds", 1, 1, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExportBehavior(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;local l=s.layers[1];l.name="paint";`+tc.setup+`;s:saveAs(s.filename)`)
			if tc.name == "tilemap" {
				// Reopen the saved fixture and prove that isTilemap, rather than
				// the non-image/group check, must reject this target.
				f.lua(p, `local tiles
for _,layer in ipairs(app.activeSprite.layers) do if layer.name=="tiles" then tiles=layer end end
assert(tiles and tiles.isImage and tiles.isTilemap and tiles.tileset)`)
			}
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "draw_pixels", Arguments: map[string]any{"sprite_path": p, "layer_name": tc.target, "frame_number": tc.frame, "pixels": []map[string]any{{"x": tc.x, "y": tc.y, "color": "#FFFFFFFF"}}}})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Contains(t, result.Content[0].(*mcp.TextContent).Text, tc.message)
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

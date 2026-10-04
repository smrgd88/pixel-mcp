//go:build integration

package tools

import (
	"context"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Exercise the intersections missing from the individual drawing/link/export
// regressions. Each Lua inspection opens the saved native file in a new process.
func TestRegressionMatrixLinkedDrawingExport(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode aseprite.ColorMode
		mask int
	}{
		{"rgb", aseprite.ColorModeRGB, 0},
		{"grayscale", aseprite.ColorModeGrayscale, 0},
		{"indexed-mask-zero", aseprite.ColorModeIndexed, 0},
		{"indexed-opaque-zero", aseprite.ColorModeIndexed, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExportBehavior(t)
			p := f.sprite(tc.mode)
			setup := fmt.Sprintf(`local s=app.activeSprite
local layer=s.layers[1];layer.name="paint"
local mask=%d
local pal=s.palettes[1];pal:resize(4)
local white=mask==0 and 3 or 0
pal:setColor(white,Color{r=255,g=255,b=255,a=255})
pal:setColor(1,Color{r=128,g=128,b=128,a=255})
pal:setColor(2,Color{r=0,g=0,b=0,a=255})
pal:setColor(mask,Color{r=255,g=0,b=255,a=255})
if s.colorMode==ColorMode.INDEXED then s.transparentColor=mask end
local function pixel(v)
 if s.colorMode==ColorMode.INDEXED then return v==255 and white or 1 end
 if s.colorMode==ColorMode.GRAY then return app.pixelColor.graya(v,255) end
 return app.pixelColor.rgba(v,v,v,255)
end
local im=Image(2,2,s.colorMode);im:clear(pixel(128))
s:newCel(layer,1,im,Point(5,3))
s:newEmptyFrame()
app.range.layers={layer};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
local group=s:newGroup();group.name="visible-group";layer.parent=group
local hidden=s:newLayer();hidden.name="hidden";hidden.isVisible=false
local cover=Image(16,16,s.colorMode);cover:clear(pixel(255))
for i=1,2 do s:newCel(hidden,i,cover,Point(0,0)) end
local hiddenGroup=s:newGroup();hiddenGroup.name="hidden-group";hiddenGroup.isVisible=false
local child=s:newLayer();child.name="hidden-child";child.parent=hiddenGroup
for i=1,2 do s:newCel(child,i,cover,Point(0,0)) end
s.data="matrix-preserved";s.frames[1].duration=0.1;s.frames[2].duration=0.2
s:saveAs(s.filename)`, tc.mask)
			f.lua(p, setup)
			// Assert preconditions after reopening; a flattened/copied fixture cannot pass.
			inspect := `local s=app.activeSprite
local layer
for _,g in ipairs(s.layers) do if g.name=="visible-group" then layer=g.layers[1] end end
assert(layer and layer.name=="paint")
assert(layer:cel(1).image==layer:cel(2).image,"native link lost")
assert(s.data=="matrix-preserved" and #s.frames==2)
assert(#s.layers==3)
for _,g in ipairs(s.layers) do
 if g.name=="hidden" then assert(not g.isVisible and g:cel(1) and g:cel(2)) end
 if g.name=="hidden-group" then assert(not g.isVisible and #g.layers==1 and g.layers[1]:cel(2)) end
end
assert(layer:cel(1).position==layer:cel(2).position)
assert(math.abs(s.frames[2].duration-0.2)<0.001)
`
			f.lua(p, inspect+`assert(layer:cel(1).position==Point(5,3))`)
			// RM-FIX-01: nested layer lookup currently rejects an existing layer.
			// Keep the reproduction and byte-preservation check until the FIX task
			// replaces this expectation with successful nested drawing.
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			rejected, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "draw_pixels", Arguments: map[string]any{"sprite_path": p, "layer_name": "paint", "frame_number": 2, "pixels": []map[string]any{{"x": 1, "y": 2, "color": "#FFFFFFFF"}}}})
			require.NoError(t, err)
			require.True(t, rejected.IsError)
			require.Contains(t, rejected.Content[0].(*mcp.TextContent).Text, "Layer not found: paint")
			unchanged, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, original, unchanged)
			f.lua(p, inspect+`layer.parent=s;s:saveAs(s.filename)`)
			// The target must be at the root so layer lookup cannot mask bounds validation.
			beforeRejectedDraw, err := os.ReadFile(p)
			require.NoError(t, err)
			result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "draw_pixels", Arguments: map[string]any{"sprite_path": p, "layer_name": "paint", "frame_number": 1, "pixels": []map[string]any{{"x": 16, "y": 0, "color": "#FFFFFFFF"}}}})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.Contains(t, result.Content[0].(*mcp.TextContent).Text, "Pixel coordinates must be within sprite bounds")
			afterRejectedDraw, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, beforeRejectedDraw, afterRejectedDraw, "out-of-bounds draw changed source")
			// Edit from the target frame and expand beyond the original cel bounds.
			f.call("draw_pixels", map[string]any{"sprite_path": p, "layer_name": "paint", "frame_number": 2, "pixels": []map[string]any{{"x": 1, "y": 2, "color": "#FFFFFFFF"}, {"x": 5, "y": 3, "color": "#FFFFFFFF"}}})
			f.lua(p, `local s=app.activeSprite;local layer,group
for _,v in ipairs(s.layers) do if v.name=="paint" then layer=v elseif v.name=="visible-group" then group=v end end
assert(layer and group);layer.parent=group;s:saveAs(s.filename)`)
			f.lua(p, inspect+fmt.Sprintf(`assert(s.colorMode==ColorMode.%s)
if s.colorMode==ColorMode.INDEXED then
 assert(s.transparentColor==%d)
 local c=layer:cel(1)
 assert(c.image:getPixel(1-c.position.x,2-c.position.y)==%d)
 assert(c.image:getPixel(2-c.position.x,2-c.position.y)==s.transparentColor)
end`, map[aseprite.ColorMode]string{aseprite.ColorModeRGB: "RGB", aseprite.ColorModeGrayscale: "GRAY", aseprite.ColorModeIndexed: "INDEXED"}[tc.mode], tc.mask, map[bool]int{true: 3, false: 0}[tc.mask == 0]))
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "matrix.png"), "format": "png", "frame_number": 0})
			files := out["files"].([]any)
			require.Len(t, files, 2)
			for i, entry := range files {
				file := entry.(map[string]any)
				require.Equal(t, float64(i+1), file["frame_number"])
				path := file["path"].(string)
				handle, err := os.Open(path)
				require.NoError(t, err)
				im, err := png.Decode(handle)
				require.NoError(t, err)
				require.NoError(t, handle.Close())
				require.Equal(t, 16, im.Bounds().Dx())
				require.Equal(t, 16, im.Bounds().Dy())
				for y := 0; y < 16; y++ {
					for x := 0; x < 16; x++ {
						want := color.NRGBA{}
						if x >= 5 && x < 7 && y >= 3 && y < 5 {
							want = color.NRGBA{128, 128, 128, 255}
						}
						if (x == 1 && y == 2) || (x == 5 && y == 3) {
							want = color.NRGBA{255, 255, 255, 255}
						}
						require.Equal(t, want, color.NRGBAModel.Convert(im.At(x, y)), "frame %d pixel %d,%d", i+1, x, y)
					}
				}
			}
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, before, after, "export changed source")

		})
	}
}

//go:build integration

package tools

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestExportOptionsBackgroundTrimPreservesOpaquePixels(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode aseprite.ColorMode
		mask int
	}{
		{"rgb", aseprite.ColorModeRGB, 0}, {"gray", aseprite.ColorModeGrayscale, 0},
		{"indexed-zero", aseprite.ColorModeIndexed, 0}, {"indexed-three", aseprite.ColorModeIndexed, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newExportOptionsFixture(t)
			p := f.sprite(tc.mode)
			f.lua(p, fmt.Sprintf(`local s=app.activeSprite
app.command.BackgroundFromLayer()
local bg=s.backgroundLayer;assert(bg)
bg.name='locked background';bg.data='keep background'
local im=bg:cel(1).image
local blue,red,green
if s.colorMode==ColorMode.INDEXED then
 s.transparentColor=%d
 local pal=s.palettes[1];pal:resize(4)
 pal:setColor(s.transparentColor,Color{r=0,g=0,b=255,a=255})
 pal:setColor(1,Color{r=255,g=0,b=0,a=255});pal:setColor(2,Color{r=0,g=255,b=0,a=255})
 blue=s.transparentColor;red=1;green=2
elseif s.colorMode==ColorMode.GRAY then
 blue=app.pixelColor.graya(64,255);red=app.pixelColor.graya(192,255);green=app.pixelColor.graya(128,255)
else
 blue=app.pixelColor.rgba(0,0,255,255);red=app.pixelColor.rgba(255,0,0,255);green=app.pixelColor.rgba(0,255,0,255)
end
im:clear(blue)
for y=5,7 do for x=3,4 do im:drawPixel(x,y,red) end end
local fg=s:newLayer();fg.name='foreground';fg.isVisible=false
local overlay=Image(2,2,s.colorMode);overlay:clear(green);s:newCel(fg,1,overlay,Point(10,11))
bg.isEditable=false;s.data='keep document';s:saveAs(s.filename)`, tc.mask))
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			stat, err := os.Stat(p)
			require.NoError(t, err)
			state := readStructure(t, f, map[string]any{"sprite_path": p})
			for _, onlyBackground := range []bool{false, true} {
				path := filepath.Join(t.TempDir(), "sheet.png")
				args := map[string]any{"sprite_path": p, "output_path": path, "layout": "horizontal", "padding": 0, "include_json": true, "trim": true}
				if onlyBackground {
					args["layer_id"] = "1"
					args["expected_revision"] = state.Revision
				}
				f.call("export_spritesheet", args)
				im := decodeExportPNG(t, path)
				require.Equal(t, image.Rect(0, 0, 16, 16), im.Bounds())
				for y := 0; y < 16; y++ {
					for x := 0; x < 16; x++ {
						want := color.NRGBA{0, 0, 255, 255}
						if tc.mode == aseprite.ColorModeGrayscale {
							want = color.NRGBA{64, 64, 64, 255}
						}
						if x >= 3 && x <= 4 && y >= 5 && y <= 7 {
							want = color.NRGBA{255, 0, 0, 255}
							if tc.mode == aseprite.ColorModeGrayscale {
								want = color.NRGBA{192, 192, 192, 255}
							}
						}
						require.Equal(t, want, color.NRGBAModel.Convert(im.At(x, y)), "%d,%d", x, y)
					}
				}
				d := readSheet(t, spritesheetDataPath(path))
				require.Len(t, d.Frames, 1)
				for _, fr := range d.Frames {
					require.False(t, fr.Trimmed)
					require.Equal(t, sheetRect{0, 0, 16, 16}, fr.Offset)
					require.Equal(t, 100, fr.Duration)
				}
			}
			// Selecting a foreground layer hides the background and must still trim.
			path := filepath.Join(t.TempDir(), "foreground.png")
			f.call("export_spritesheet", map[string]any{"sprite_path": p, "output_path": path, "layout": "horizontal", "padding": 0, "include_json": true, "trim": true, "layer_id": "2", "expected_revision": state.Revision, "include_hidden": true})
			im := decodeExportPNG(t, path)
			require.Equal(t, image.Rect(0, 0, 2, 2), im.Bounds())
			d := readSheet(t, spritesheetDataPath(path))
			for _, fr := range d.Frames {
				require.True(t, fr.Trimmed)
				require.Equal(t, sheetRect{10, 11, 2, 2}, fr.Offset)
			}
			sourceUnchanged(t, p, before, stat)
			require.Empty(t, historyOperations(f, p))
			f.lua(p, `local s=app.activeSprite;assert(s.backgroundLayer and not s.backgroundLayer.isEditable);assert(s.backgroundLayer.name=='locked background' and s.backgroundLayer.data=='keep background' and s.data=='keep document')`)
		})
	}
}

func TestExportOptionsPaddedAxesRejectedBeforeStaging(t *testing.T) {
	f := newExportOptionsFixture(t)
	for _, vertical := range []bool{false, true} {
		t.Run(fmt.Sprint(vertical), func(t *testing.T) {
			w, h, layout := 1, 32767, "horizontal"
			if vertical {
				w, h, layout = 32767, 1, "vertical"
			}
			p := filepath.Join(t.TempDir(), "edge.aseprite")
			f.lua("", fmt.Sprintf(`local s=Sprite(%d,%d);s:saveAs("%s")`, w, h, aseprite.EscapeString(p)))
			for _, option := range []string{"border", "inner", "extrude"} {
				// Invalid dimensions must be rejected before even creating output parents.
				dest := filepath.Join(t.TempDir(), "not-created", "sheet.png")
				one := 1
				in := ExportSpritesheetInput{SpritePath: p, OutputPath: dest, Layout: layout}
				switch option {
				case "border":
					in.BorderPadding = &one
				case "inner":
					in.InnerPadding = &one
				case "extrude":
					in.Extrude = true
				}
				_, err := exportSpritesheet(context.Background(), f.client, f.gen, in)
				require.Error(t, err)
				require.Equal(t, "invalid_arguments", diagnostics.Classify(err).Code)
				_, err = os.Stat(filepath.Dir(dest))
				require.True(t, os.IsNotExist(err), "native export/staging must not start")
			}
		})
	}
}

func TestExportOptionsBackgroundLinkedPalettes(t *testing.T) {
	for _, perFrame := range []bool{false, true} {
		t.Run(fmt.Sprint(perFrame), func(t *testing.T) {
			f := newExportOptionsFixture(t)
			p := f.sprite(aseprite.ColorModeIndexed)
			f.lua(p, `local s=app.activeSprite
app.command.BackgroundFromLayer()
local bg=s.backgroundLayer;s.transparentColor=0
local pal=s.palettes[1];pal:resize(4);pal:setColor(0,Color{r=0,g=0,b=255,a=255})
bg:cel(1).image:clear(0)
s:newFrame();s.frames[2].duration=0.2
app.range.layers={bg};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
s.data='linked background';s:saveAs(s.filename)`)
			if perFrame {
				b, err := os.ReadFile(p)
				require.NoError(t, err)
				start := 128 + int(binary.LittleEndian.Uint32(b[128:]))
				chunk := make([]byte, 32)
				binary.LittleEndian.PutUint32(chunk, 32)
				binary.LittleEndian.PutUint16(chunk[4:], 0x2019)
				binary.LittleEndian.PutUint32(chunk[6:], 4)
				copy(chunk[28:], []byte{0, 255, 0, 255})
				binary.LittleEndian.PutUint32(b[start:], binary.LittleEndian.Uint32(b[start:])+32)
				binary.LittleEndian.PutUint16(b[start+6:], binary.LittleEndian.Uint16(b[start+6:])+1)
				b = append(b[:start+16], append(chunk, b[start+16:]...)...)
				binary.LittleEndian.PutUint32(b, uint32(len(b)))
				require.NoError(t, os.WriteFile(p, b, 0600))
			}
			f.lua(p, `local s=app.activeSprite;assert(s.backgroundLayer:cel(1).image==s.backgroundLayer:cel(2).image);assert(s.backgroundLayer:cel(1).image:getPixel(0,0)==s.transparentColor)`)
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			stat, err := os.Stat(p)
			require.NoError(t, err)
			for _, trim := range []bool{false, true} {
				path := filepath.Join(t.TempDir(), "sheet.png")
				f.call("export_spritesheet", map[string]any{"sprite_path": p, "output_path": path, "layout": "horizontal", "padding": 0, "include_json": true, "trim": trim, "extrude": true, "border_padding": 2, "shape_padding": 1})
				d := readSheet(t, spritesheetDataPath(path))
				im := decodeExportPNG(t, path)
				require.Len(t, d.Frames, 2)
				for _, fr := range d.Frames {
					require.Equal(t, sheetRect{0, 0, 16, 16}, fr.Offset)
					require.False(t, fr.Trimmed)
					require.Equal(t, 100*fr.Number, fr.Duration)
					want := color.NRGBA{0, 0, 255, 255}
					if perFrame && fr.Number == 2 {
						want = color.NRGBA{0, 255, 0, 255}
					}
					// Include the extruded rim: transparent-index background pixels are opaque.
					for y := fr.Frame.Y - 1; y <= fr.Frame.Y+fr.Frame.H; y++ {
						for x := fr.Frame.X - 1; x <= fr.Frame.X+fr.Frame.W; x++ {
							require.Equal(t, want, color.NRGBAModel.Convert(im.At(x, y)))
						}
					}
				}
			}
			sourceUnchanged(t, p, before, stat)
			f.lua(p, `local s=app.activeSprite;assert(s.colorMode==ColorMode.INDEXED and s.backgroundLayer and s.data=='linked background');assert(s.backgroundLayer:cel(1).image==s.backgroundLayer:cel(2).image)`)
		})
	}
}

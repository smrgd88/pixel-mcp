//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

type behaviorFixture struct {
	t       *testing.T
	session *mcp.ClientSession
	client  *aseprite.Client
	gen     *aseprite.LuaGenerator
}

func newBehaviorFixture(t *testing.T) *behaviorFixture {
	t.Helper()
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, cfg.Timeout)
	gen := aseprite.NewLuaGenerator()
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	server := mcp.NewServer(&mcp.Implementation{Name: "behavior-regression", Version: "1"}, nil)
	RegisterCanvasTools(server, client, gen, cfg, logger)
	RegisterDrawingTools(server, client, gen, cfg, logger)
	RegisterAnimationTools(server, client, gen, cfg, logger)
	RegisterSelectionTools(server, client, gen, cfg, logger)
	RegisterDitheringTools(server, client, gen, cfg, logger)
	RegisterQuantizationTools(server, client, gen, cfg, logger)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &behaviorFixture{t, cs, client, gen}
}
func (f *behaviorFixture) call(name string, a map[string]any) map[string]any {
	f.t.Helper()
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: a})
	require.NoError(f.t, err)
	require.False(f.t, r.IsError, "%s: %+v", name, r.Content[0].(*mcp.TextContent).Text)
	var out map[string]any
	require.NoError(f.t, json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &out))
	return out
}
func (f *behaviorFixture) lua(p, code string) string {
	f.t.Helper()
	out, err := f.client.ExecuteLua(context.Background(), code, p)
	require.NoError(f.t, err, "%s", out)
	return out
}
func (f *behaviorFixture) sprite(mode aseprite.ColorMode) string {
	f.t.Helper()
	p := filepath.Join(f.t.TempDir(), "sprite.aseprite")
	f.lua("", f.gen.CreateCanvas(16, 16, mode, p))
	return p
}
func TestBehavior_IndexedPixelsWithoutPaletteFlag(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeIndexed)
	f.lua(p, f.gen.SetPalette([]string{"#000000", "#FFFFFF", "#FF0000"}))
	f.lua(p, f.gen.AddLayer("New"))
	f.call("draw_pixels", map[string]any{"sprite_path": p, "layer_name": "New", "frame_number": 1, "pixels": []map[string]any{{"x": 10, "y": 11, "color": "#FFFFFF"}, {"x": 1, "y": 2, "color": "#FFFFFF"}}})
	f.lua(p, `local s=app.activeSprite;local c=s.layers[2]:cel(1);for _,p in ipairs({{10,11},{1,2}}) do local i=c.image:getPixel(p[1]-c.position.x,p[2]-c.position.y);assert(i < #s.palettes[1]);assert(i~=s.transparentColor);assert(s.palettes[1]:getColor(i).red==255) end`)
}
func TestBehavior_DitherPatternsRetainBothColors(t *testing.T) {
	for _, pattern := range []string{"checkerboard", "grass", "water", "stone", "cloud", "brick", "dots", "diagonal", "cross", "noise", "horizontal_lines", "vertical_lines", "bayer_2x2", "bayer_4x4", "bayer_8x8"} {
		t.Run(pattern, func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.call("draw_with_dither", map[string]any{"sprite_path": p, "layer_name": "Layer 1", "frame_number": 1, "region": map[string]int{"x": 0, "y": 0, "width": 16, "height": 16}, "color1": "#000000", "color2": "#FFFFFF", "pattern": pattern, "density": 0.5})
			f.lua(p, `local i=app.activeSprite.layers[1]:cel(1).image;local black,white=0,0;for y=0,15 do for x=0,15 do local v=i:getPixel(x,y);assert(app.pixelColor.rgbaA(v)==255);if app.pixelColor.rgbaR(v)==0 then black=black+1 else white=white+1 end end end;assert(black>0 and white>0)`)
		})
	}
}
func TestBehavior_SelectionMaskAndCombination(t *testing.T) {
	for _, kind := range []string{"all", "ellipse", "subtract", "intersect", "empty", "move-negative"} {
		t.Run(kind, func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, f.gen.DrawRectangle("Layer 1", 1, 0, 0, 16, 16, aseprite.Color{R: 255, A: 255}, true, false))
			args := map[string]any{"sprite_path": p, "x": 2, "y": 2, "width": 6, "height": 6, "mode": "replace"}
			switch kind {
			case "all":
				f.call("select_all", map[string]any{"sprite_path": p})
			case "ellipse", "move-negative":
				f.call("select_ellipse", args)
			default:
				f.call("select_rectangle", args)
			}
			if kind == "move-negative" {
				f.call("move_selection", map[string]any{"sprite_path": p, "dx": -1, "dy": -1})
			}
			if kind == "subtract" || kind == "intersect" || kind == "empty" {
				mode := kind
				if kind == "empty" {
					mode = "subtract"
				} else {
					args["width"] = 3
				}
				args["mode"] = mode
				f.call("select_rectangle", args)
			}
			if kind == "empty" {
				f.lua(p, `assert(app.activeSprite.data=="")`)
				return
			}
			f.call("copy_selection", map[string]any{"sprite_path": p})
			switch kind {
			case "all":
				f.lua(p, `local s=app.activeSprite;local d=json.decode(s.data);assert(d.selection.w==16 and d.selection.h==16);assert(s.layers[2]:cel(1).image.width==16)`)
			case "ellipse", "move-negative":
				f.lua(p, `local s=app.activeSprite;local d=json.decode(s.data);assert(d.selection.w==6 and d.selection.h==6);local i=s.layers[2]:cel(1).image;assert(app.pixelColor.rgbaA(i:getPixel(0,0))==0);assert(app.pixelColor.rgbaA(i:getPixel(3,3))==255)`)
			case "subtract":
				f.lua(p, `local d=json.decode(app.activeSprite.data);assert(d.selection.x==5 and d.selection.w==3)`)
			case "intersect":
				f.lua(p, `local d=json.decode(app.activeSprite.data);assert(d.selection.x==2 and d.selection.w==3)`)
			}
		})
	}
}
func TestBehavior_OffsetClipboardAndPaste(t *testing.T) {
	for _, cut := range []bool{false, true} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;local i=Image(2,2,ColorMode.RGB);i:drawPixel(0,0,app.pixelColor.rgba(255,0,0,255));s:newCel(s.layers[1],1,i,Point(7,8));s:saveAs(s.filename)`)
			f.call("select_rectangle", map[string]any{"sprite_path": p, "x": 7, "y": 8, "width": 1, "height": 1, "mode": "replace"})
			if cut {
				f.call("cut_selection", map[string]any{"sprite_path": p, "layer_name": "Layer 1", "frame_number": 1})
			} else {
				f.call("copy_selection", map[string]any{"sprite_path": p})
			}
			f.lua(p, `local c=app.activeSprite.layers[2]:cel(1);assert(c.position.x==7 and c.position.y==8);assert(app.pixelColor.rgbaR(c.image:getPixel(0,0))==255)`)
			f.call("paste_clipboard", map[string]any{"sprite_path": p, "layer_name": "Layer 1", "frame_number": 1, "x": 1, "y": 2})
			f.lua(p, `local c=app.activeSprite.layers[1]:cel(1);assert(app.pixelColor.rgbaR(c.image:getPixel(1-c.position.x,2-c.position.y))==255)`)
		})
	}
}
func TestBehavior_QuantizationTransparencyAndMode(t *testing.T) {
	for _, algorithm := range []string{"median_cut", "kmeans", "octree"} {
		for _, indexed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/indexed=%v", algorithm, indexed), func(t *testing.T) {
				f := newBehaviorFixture(t)
				p := f.sprite(aseprite.ColorModeRGB)
				f.lua(p, f.gen.DrawRectangle("Layer 1", 1, 4, 4, 8, 8, aseprite.Color{R: 170, G: 100, B: 60, A: 255}, true, false))
				r := f.call("quantize_palette", map[string]any{"sprite_path": p, "target_colors": 4, "algorithm": algorithm, "dither": true, "preserve_transparency": true, "convert_to_indexed": indexed})
				expected := "rgb"
				if indexed {
					expected = "indexed"
				}
				require.Equal(t, expected, r["color_mode"])
				f.lua(p, `local s=app.activeSprite;local c=s.layers[1]:cel(1);local outside=c.image:getPixel(0-c.position.x,0-c.position.y);local inside=c.image:getPixel(5-c.position.x,5-c.position.y);if s.colorMode==ColorMode.INDEXED then assert(outside==s.transparentColor);assert(inside~=s.transparentColor) else assert(app.pixelColor.rgbaA(outside)==0);assert(app.pixelColor.rgbaA(inside)==255) end`)
			})
		}
	}
}

func TestBehavior_DuplicateFrameAppendsAndCopiesIndependently(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, f.gen.DrawPixels("Layer 1", 1, []aseprite.Pixel{{Point: aseprite.Point{X: 1, Y: 1}, Color: aseprite.Color{R: 255, A: 255}}}, false))
	f.call("duplicate_frame", map[string]any{"sprite_path": p, "source_frame": 1, "insert_after": 0})
	f.lua(p, f.gen.DrawPixels("Layer 1", 2, []aseprite.Pixel{{Point: aseprite.Point{X: 1, Y: 1}, Color: aseprite.Color{G: 255, A: 255}}}, false))
	r := f.call("duplicate_frame", map[string]any{"sprite_path": p, "source_frame": 1, "insert_after": 0})
	require.Equal(t, float64(3), r["new_frame_number"])
	f.lua(p, `local s=app.activeSprite;local l=s.layers[1];local function pixel(n) local c=l:cel(n);return c.image:getPixel(1-c.position.x,1-c.position.y) end;assert(app.pixelColor.rgbaG(pixel(2))==255);assert(app.pixelColor.rgbaR(pixel(3))==255)`)
}

func TestBehavior_SelectionPreservesMetadataAndEmptyMask(t *testing.T) {
	for _, data := range []string{`{"author":"artist"}`, `custom artist data`} {
		t.Run(data, func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, fmt.Sprintf(`app.activeSprite.data="%s";app.activeSprite:saveAs(app.activeSprite.filename)`, aseprite.EscapeString(data)))
			f.call("select_all", map[string]any{"sprite_path": p})
			f.call("deselect", map[string]any{"sprite_path": p})
			if data[0] == '{' {
				f.lua(p, `assert(json.decode(app.activeSprite.data).author=="artist")`)
			} else {
				f.lua(p, `assert(app.activeSprite.data=="custom artist data")`)
			}
		})
	}
}

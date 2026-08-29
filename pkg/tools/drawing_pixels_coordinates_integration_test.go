//go:build integration

package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestIntegration_DrawPixels_UsesSpriteCoordinatesForPositionedCel(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-draw-pixels-positioned-cel.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(32, 32, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}

	shapeColor := aseprite.Color{R: 0, G: 170, B: 255, A: 255}
	if _, err := client.ExecuteLua(ctx, gen.DrawRectangle("Layer 1", 1, 10, 6, 8, 8, shapeColor, true, false), spritePath); err != nil {
		t.Fatalf("draw rectangle: %v", err)
	}

	celX, celY := readCelPosition(t, ctx, client, spritePath, "Layer 1", 1)
	if celX == 0 || celY == 0 || celX == celY {
		t.Fatalf("test setup did not produce an asymmetric non-zero cel position: (%d,%d)", celX, celY)
	}
	t.Logf("verified precondition: cel position is (%d,%d)", celX, celY)

	pixels := []aseprite.Pixel{
		{Point: aseprite.Point{X: 12, Y: 8}, Color: aseprite.Color{R: 255, G: 0, B: 0, A: 255}},
		{Point: aseprite.Point{X: 1, Y: 2}, Color: aseprite.Color{R: 0, G: 255, B: 0, A: 255}},
	}
	if _, err := client.ExecuteLua(ctx, gen.DrawPixels("Layer 1", 1, pixels, false), spritePath); err != nil {
		t.Fatalf("draw pixels: %v", err)
	}

	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, 12, 8, "#FF0000FF")
	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, 1, 2, "#00FF00FF")
}

func TestIntegration_DrawPixels_UsesSpriteCoordinatesForPositionedIndexedCel(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-draw-pixels-positioned-indexed-cel.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(32, 32, aseprite.ColorModeIndexed, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.SetPalette([]string{"#FF0000FF", "#00FF00FF", "#00000000"}), spritePath); err != nil {
		t.Fatalf("set palette: %v", err)
	}

	setup := `local spr = app.activeSprite
local cel = spr.layers[1]:cel(spr.frames[1])
local image = Image(8, 8, ColorMode.INDEXED)
image:clear(0)
cel.image = image
cel.position = Point(10, 6)
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, setup, spritePath); err != nil {
		t.Fatalf("position indexed cel: %v", err)
	}
	celX, celY := readCelPosition(t, ctx, client, spritePath, "Layer 1", 1)
	if celX == 0 || celY == 0 {
		t.Fatalf("test setup did not produce a non-zero indexed cel position: (%d,%d)", celX, celY)
	}

	pixels := []aseprite.Pixel{{
		Point: aseprite.Point{X: 1, Y: 2},
		Color: aseprite.Color{R: 0, G: 255, B: 0, A: 255},
	}}
	if _, err := client.ExecuteLua(ctx, gen.DrawPixels("Layer 1", 1, pixels, true), spritePath); err != nil {
		t.Fatalf("draw pixels: %v", err)
	}

	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, 1, 2, "#00FF00FF")
	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, 12, 8, "#FF0000FF")
	assertIndexedPixelTransparent(t, ctx, client, spritePath, "Layer 1", 1, 30, 30)
}

func TestIntegration_DrawPixels_PreservesOffCanvasCelContent(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-draw-pixels-off-canvas.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(32, 32, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}

	setup := `local spr = app.activeSprite
local cel = spr.layers[1]:cel(spr.frames[1])
local image = Image(8, 8, ColorMode.RGB)
image:clear(Color(0, 0, 0, 0))
image:putPixel(0, 0, Color(255, 0, 0, 255))
image:putPixel(4, 4, Color(0, 0, 255, 255))
cel.image = image
cel.position = Point(-2, -2)
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, setup, spritePath); err != nil {
		t.Fatalf("position off-canvas cel: %v", err)
	}

	pixels := []aseprite.Pixel{{
		Point: aseprite.Point{X: 10, Y: 10},
		Color: aseprite.Color{R: 0, G: 255, B: 0, A: 255},
	}}
	if _, err := client.ExecuteLua(ctx, gen.DrawPixels("Layer 1", 1, pixels, false), spritePath); err != nil {
		t.Fatalf("draw pixels: %v", err)
	}

	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, -2, -2, "#FF0000FF")
	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, 2, 2, "#0000FFFF")
	assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", 1, 10, 10, "#00FF00FF")
}

func TestIntegration_DrawPixels_PreservesNativeLinkedCels(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-draw-pixels-linked-cels.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(32, 32, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	shapeColor := aseprite.Color{R: 0, G: 170, B: 255, A: 255}
	if _, err := client.ExecuteLua(ctx, gen.DrawRectangle("Layer 1", 1, 10, 6, 8, 8, shapeColor, true, false), spritePath); err != nil {
		t.Fatalf("draw rectangle: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.AddFrame(100), spritePath); err != nil {
		t.Fatalf("add frame: %v", err)
	}

	link := `local spr = app.activeSprite
local layer = spr.layers[1]
local frame2 = spr.frames[2]
if not layer:cel(frame2) then
	spr:newCel(layer, frame2, Image(spr.spec), Point(0, 0))
end
app.range.layers = { layer }
app.range.frames = { spr.frames[1], spr.frames[2] }
app.command.LinkCels()
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, link, spritePath); err != nil {
		t.Fatalf("link cels: %v", err)
	}

	verifyLink := `local spr = app.activeSprite
local layer = spr.layers[1]
local first = layer:cel(spr.frames[1])
local second = layer:cel(spr.frames[2])
print(tostring(first.image == second.image))`
	linked, err := client.ExecuteLua(ctx, verifyLink, spritePath)
	if err != nil {
		t.Fatalf("inspect linked cels: %v", err)
	}
	if strings.TrimSpace(linked) != "true" {
		t.Fatal("test setup did not produce native linked cels")
	}

	pixels := []aseprite.Pixel{{
		Point: aseprite.Point{X: 1, Y: 2},
		Color: aseprite.Color{R: 0, G: 255, B: 0, A: 255},
	}}
	if _, err := client.ExecuteLua(ctx, gen.DrawPixels("Layer 1", 1, pixels, false), spritePath); err != nil {
		t.Fatalf("draw pixels: %v", err)
	}

	for frame := 1; frame <= 2; frame++ {
		assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", frame, 1, 2, "#00FF00FF")
		assertPixelRGBA(t, ctx, client, gen, spritePath, "Layer 1", frame, 12, 8, "#00AAFFFF")
	}
}

func TestIntegration_DrawPixels_RejectsCoordinatesOutsideSprite(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-draw-pixels-outside-sprite.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(32, 32, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}

	pixels := []aseprite.Pixel{{
		Point: aseprite.Point{X: 32, Y: 0},
		Color: aseprite.Color{R: 255, G: 0, B: 0, A: 255},
	}}
	_, err := client.ExecuteLua(ctx, gen.DrawPixels("Layer 1", 1, pixels, false), spritePath)
	if err == nil {
		t.Fatal("draw_pixels accepted a coordinate outside the sprite")
	}
	if !strings.Contains(err.Error(), "Pixel coordinates must be within sprite bounds") {
		t.Fatalf("draw_pixels error = %v", err)
	}
}

func readCelPosition(t *testing.T, ctx context.Context, client *aseprite.Client, spritePath, layerName string, frameNumber int) (int, int) {
	t.Helper()
	script := fmt.Sprintf(`local spr = app.activeSprite
local layer = nil
for _, candidate in ipairs(spr.layers) do
	if candidate.name == "%s" then layer = candidate break end
end
local cel = layer and layer:cel(spr.frames[%d])
if not cel then error("No cel") end
print(string.format("%%d,%%d", cel.position.x, cel.position.y))`, aseprite.EscapeString(layerName), frameNumber)

	output, err := client.ExecuteLua(ctx, script, spritePath)
	if err != nil {
		t.Fatalf("read cel position: %v", err)
	}
	var x, y int
	if _, err := fmt.Sscanf(strings.TrimSpace(output), "%d,%d", &x, &y); err != nil {
		t.Fatalf("parse cel position %q: %v", output, err)
	}
	return x, y
}

func assertPixelRGBA(t *testing.T, ctx context.Context, client *aseprite.Client, gen *aseprite.LuaGenerator, spritePath, layerName string, frameNumber, x, y int, want string) {
	t.Helper()
	output, err := client.ExecuteLua(ctx, gen.GetPixels(layerName, frameNumber, x, y, 1, 1), spritePath)
	if err != nil {
		t.Fatalf("get pixel (%d,%d): %v", x, y, err)
	}
	pixels, err := testutil.ParsePixelData(output)
	if err != nil {
		t.Fatalf("parse pixel (%d,%d): %v", x, y, err)
	}
	if len(pixels) != 1 {
		t.Fatalf("get pixel (%d,%d) returned %d pixels, want 1", x, y, len(pixels))
	}
	if pixels[0].X != x || pixels[0].Y != y || pixels[0].Color != want {
		t.Errorf("pixel = (%d,%d,%s), want (%d,%d,%s)", pixels[0].X, pixels[0].Y, pixels[0].Color, x, y, want)
	}
}

func assertIndexedPixelTransparent(t *testing.T, ctx context.Context, client *aseprite.Client, spritePath, layerName string, frameNumber, x, y int) {
	t.Helper()
	script := fmt.Sprintf(`local spr = app.activeSprite
local layer = nil
for _, candidate in ipairs(spr.layers) do
	if candidate.name == "%s" then layer = candidate break end
end
local cel = layer and layer:cel(spr.frames[%d])
if not cel then print("transparent") return end
local imageX = %d - cel.position.x
local imageY = %d - cel.position.y
if imageX < 0 or imageX >= cel.image.width or imageY < 0 or imageY >= cel.image.height then
	print("transparent")
	return
end
if cel.image:getPixel(imageX, imageY) == spr.transparentColor then
	print("transparent")
else
	print("opaque")
end`, aseprite.EscapeString(layerName), frameNumber, x, y)

	output, err := client.ExecuteLua(ctx, script, spritePath)
	if err != nil {
		t.Fatalf("inspect indexed pixel (%d,%d): %v", x, y, err)
	}
	if strings.TrimSpace(output) != "transparent" {
		t.Errorf("indexed pixel (%d,%d) is not transparent", x, y)
	}
}

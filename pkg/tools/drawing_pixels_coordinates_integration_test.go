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

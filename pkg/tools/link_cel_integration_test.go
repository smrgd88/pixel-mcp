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

func TestIntegration_LinkCel_NativeLinkPersistsAndPropagates(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()
	spritePath := testutil.TempSpritePath(t, "link-cel-native.aseprite")

	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(16, 16, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	setup := `local spr = app.activeSprite
local layer = spr.layers[1]
layer.name = "Layer 1"
local source = layer:cel(spr.frames[1])
source.position = Point(3, 4)
source.image:drawPixel(0, 0, Color(255, 0, 0, 255))
spr:newEmptyFrame()
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, setup, spritePath); err != nil {
		t.Fatalf("set up source cel: %v", err)
	}

	if _, err := client.ExecuteLua(ctx, gen.LinkCel("Layer 1", 1, 2), spritePath); err != nil {
		t.Fatalf("link cel: %v", err)
	}

	assertLinked := `local spr = app.activeSprite
local layer = spr.layers[1]
local source = layer:cel(spr.frames[1])
local target = layer:cel(spr.frames[2])
print(string.format("%s|%d,%d|%d,%d", tostring(source.image == target.image), source.position.x, source.position.y, target.position.x, target.position.y))`
	output, err := client.ExecuteLua(ctx, assertLinked, spritePath)
	if err != nil {
		t.Fatalf("inspect reopened sprite: %v", err)
	}
	if got := strings.TrimSpace(output); got != "true|3,4|3,4" {
		t.Fatalf("reopened cels are not native-linked with preserved positions: %s", got)
	}

	mutateSource := `local spr = app.activeSprite
local layer = spr.layers[1]
layer:cel(spr.frames[1]).image:drawPixel(1, 1, Color(0, 255, 0, 255))
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, mutateSource, spritePath); err != nil {
		t.Fatalf("mutate source: %v", err)
	}
	assertPixelValue(t, ctx, client, spritePath, 2, 1, 1, appRGBA(0, 255, 0, 255))

	mutateTarget := `local spr = app.activeSprite
local layer = spr.layers[1]
layer:cel(spr.frames[2]).image:drawPixel(2, 2, Color(0, 0, 255, 255))
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, mutateTarget, spritePath); err != nil {
		t.Fatalf("mutate target: %v", err)
	}
	assertPixelValue(t, ctx, client, spritePath, 1, 2, 2, appRGBA(0, 0, 255, 255))
}

func TestIntegration_LinkCel_RejectsExistingTargetWithoutChanges(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()
	spritePath := testutil.TempSpritePath(t, "link-cel-existing-target.aseprite")

	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(16, 16, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	setup := `local spr = app.activeSprite
local layer = spr.layers[1]
layer.name = "Layer 1"
local source = layer:cel(spr.frames[1])
source.position = Point(3, 4)
source.image:drawPixel(0, 0, Color(255, 0, 0, 255))
local frame2 = spr:newEmptyFrame()
local target = spr:newCel(layer, frame2, Image(spr.spec), Point(6, 7))
target.image:drawPixel(0, 0, Color(0, 0, 255, 255))
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, setup, spritePath); err != nil {
		t.Fatalf("set up existing target: %v", err)
	}

	if _, err := client.ExecuteLua(ctx, gen.LinkCel("Layer 1", 1, 2), spritePath); err == nil {
		t.Fatal("link cel succeeded with an existing target cel")
	}

	inspect := `local spr = app.activeSprite
local layer = spr.layers[1]
local source = layer:cel(spr.frames[1])
local target = layer:cel(spr.frames[2])
print(string.format("%s|%d,%d|%d,%d|%08X|%08X", tostring(source.image == target.image), source.position.x, source.position.y, target.position.x, target.position.y, source.image:getPixel(0, 0), target.image:getPixel(0, 0)))`
	output, err := client.ExecuteLua(ctx, inspect, spritePath)
	if err != nil {
		t.Fatalf("inspect unchanged sprite: %v", err)
	}
	want := "false|3,4|6,7|" + appRGBA(255, 0, 0, 255) + "|" + appRGBA(0, 0, 255, 255)
	if got := strings.TrimSpace(output); got != want {
		t.Fatalf("failed link changed the original sprite: got %s, want %s", got, want)
	}
}

func TestIntegration_LinkCel_BackwardsFrameOrder(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()
	spritePath := testutil.TempSpritePath(t, "link-cel-backwards.aseprite")

	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(16, 16, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	setup := `local spr = app.activeSprite
local layer = spr.layers[1]
layer.name = "Layer 1"
spr:newEmptyFrame()
local source = layer:cel(spr.frames[1])
source.frame = spr.frames[2]
source.position = Point(5, 6)
source.image:drawPixel(0, 0, Color(255, 128, 0, 255))
spr:saveAs(spr.filename)`
	if _, err := client.ExecuteLua(ctx, setup, spritePath); err != nil {
		t.Fatalf("set up later source: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.LinkCel("Layer 1", 2, 1), spritePath); err != nil {
		t.Fatalf("link backwards: %v", err)
	}
	inspect := `local spr = app.activeSprite
local layer = spr.layers[1]
local target = layer:cel(spr.frames[1])
local source = layer:cel(spr.frames[2])
print(string.format("%s|%d,%d", tostring(source.image == target.image), target.position.x, target.position.y))`
	output, err := client.ExecuteLua(ctx, inspect, spritePath)
	if err != nil {
		t.Fatalf("inspect backwards link: %v", err)
	}
	if got := strings.TrimSpace(output); got != "true|5,6" {
		t.Fatalf("backwards link = %s, want true|5,6", got)
	}
}

func assertPixelValue(t *testing.T, ctx context.Context, client *aseprite.Client, spritePath string, frame, x, y int, want string) {
	t.Helper()
	script := fmt.Sprintf(`local spr = app.activeSprite
local pixel = spr.layers[1]:cel(spr.frames[%d]).image:getPixel(%d, %d)
print(string.format("%%08X", pixel))`, frame, x, y)
	output, err := client.ExecuteLua(ctx, script, spritePath)
	if err != nil {
		t.Fatalf("inspect pixel: %v", err)
	}
	if got := strings.TrimSpace(output); got != want {
		t.Fatalf("frame %d pixel (%d,%d) = %s, want %s", frame, x, y, got, want)
	}
}

func appRGBA(r, g, b, a uint32) string {
	// Aseprite's RGB pixel value is ABGR on little-endian builds.
	value := r | g<<8 | b<<16 | a<<24
	return fmt.Sprintf("%08X", value)
}

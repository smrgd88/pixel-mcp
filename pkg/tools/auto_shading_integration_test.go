//go:build integration
// +build integration

package tools

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Integration tests for auto-shading tools with real Aseprite.
// Run with: go test -tags=integration -v ./pkg/tools -run TestIntegration_AutoShading

func TestIntegration_ApplyAutoShading_CellStyle(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	// Create a canvas with a simple shape
	spritePath := testutil.TempSpritePath(t, "test-autoshading-cell.aseprite")
	createScript := gen.CreateCanvas(64, 64, aseprite.ColorModeRGB, spritePath)
	_, err := client.ExecuteLua(ctx, createScript, "")
	if err != nil {
		t.Fatalf("Failed to create canvas: %v", err)
	}
	defer os.Remove(spritePath)

	// Draw a circle to shade
	drawScript := gen.DrawCircle("Layer 1", 1, 32, 32, 20, aseprite.Color{R: 255, G: 128, B: 0, A: 255}, true, false)
	_, err = client.ExecuteLua(ctx, drawScript, spritePath)
	if err != nil {
		t.Fatalf("Failed to draw circle: %v", err)
	}

	// Export layer to PNG
	tempPNG := testutil.TempSpritePath(t, "layer.png")
	defer os.Remove(tempPNG)

	exportScript := gen.ExportSprite(tempPNG, 1)
	_, err = client.ExecuteLua(ctx, exportScript, spritePath)
	if err != nil {
		t.Fatalf("Failed to export layer: %v", err)
	}

	// Load PNG
	imgFile, err := os.Open(tempPNG)
	if err != nil {
		t.Fatalf("Failed to open PNG: %v", err)
	}
	defer imgFile.Close()

	img, err := png.Decode(imgFile)
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Apply auto-shading with cell style
	shadedImg, generatedColors, regionsShadedCount, err := aseprite.ApplyAutoShading(
		img,
		"top_left",
		0.6,
		"cell",
		true,
	)
	if err != nil {
		t.Fatalf("Auto-shading failed: %v", err)
	}

	if shadedImg == nil {
		t.Fatal("Shaded image should not be nil")
	}

	if len(generatedColors) == 0 {
		t.Error("Generated colors should not be empty")
	}

	if regionsShadedCount == 0 {
		t.Error("Regions shaded count should not be 0")
	}

	t.Logf("✓ Applied cell shading: %d colors generated, %d regions shaded", len(generatedColors), regionsShadedCount)
}

func TestIntegration_ApplyAutoShading_SmoothStyle(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	// Create a canvas
	spritePath := testutil.TempSpritePath(t, "test-autoshading-smooth.aseprite")
	createScript := gen.CreateCanvas(64, 64, aseprite.ColorModeRGB, spritePath)
	_, err := client.ExecuteLua(ctx, createScript, "")
	if err != nil {
		t.Fatalf("Failed to create canvas: %v", err)
	}
	defer os.Remove(spritePath)

	// Draw a rectangle
	drawScript := gen.DrawRectangle("Layer 1", 1, 16, 16, 32, 32, aseprite.Color{R: 0, G: 128, B: 255, A: 255}, true, false)
	_, err = client.ExecuteLua(ctx, drawScript, spritePath)
	if err != nil {
		t.Fatalf("Failed to draw rectangle: %v", err)
	}

	// Export and shade
	tempPNG := testutil.TempSpritePath(t, "rect.png")
	defer os.Remove(tempPNG)

	exportScript := gen.ExportSprite(tempPNG, 1)
	_, err = client.ExecuteLua(ctx, exportScript, spritePath)
	if err != nil {
		t.Fatalf("Failed to export: %v", err)
	}

	imgFile, err := os.Open(tempPNG)
	if err != nil {
		t.Fatalf("Failed to open PNG: %v", err)
	}
	defer imgFile.Close()

	img, err := png.Decode(imgFile)
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Apply auto-shading with smooth style
	shadedImg, generatedColors, regionsShadedCount, err := aseprite.ApplyAutoShading(
		img,
		"top",
		0.5,
		"smooth",
		false, // No hue shift
	)
	if err != nil {
		t.Fatalf("Auto-shading failed: %v", err)
	}

	if shadedImg == nil {
		t.Fatal("Shaded image should not be nil")
	}

	t.Logf("✓ Applied smooth shading: %d colors generated, %d regions shaded", len(generatedColors), regionsShadedCount)
}

func TestIntegration_ApplyAutoShading_SoftStyle(t *testing.T) {
	// Create a simple test image
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 255, A: 255})
		}
	}

	// Apply auto-shading with soft style
	shadedImg, generatedColors, regionsShadedCount, err := aseprite.ApplyAutoShading(
		img,
		"bottom_right",
		0.3,
		"soft",
		true,
	)
	if err != nil {
		t.Fatalf("Auto-shading failed: %v", err)
	}

	if shadedImg == nil {
		t.Fatal("Shaded image should not be nil")
	}

	if len(generatedColors) == 0 {
		t.Error("Generated colors should not be empty")
	}

	t.Logf("✓ Applied soft shading: %d colors generated, %d regions shaded", len(generatedColors), regionsShadedCount)
}

func TestIntegration_ApplyAutoShading_AllLightDirections(t *testing.T) {
	directions := []string{
		"top_left", "top", "top_right",
		"left", "right",
		"bottom_left", "bottom", "bottom_right",
	}

	for _, dir := range directions {
		t.Run(dir, func(t *testing.T) {
			// Create a simple test image
			img := image.NewRGBA(image.Rect(0, 0, 32, 32))
			for y := 8; y < 24; y++ {
				for x := 8; x < 24; x++ {
					img.SetRGBA(x, y, color.RGBA{R: 100, G: 200, B: 100, A: 255})
				}
			}

			// Apply auto-shading
			_, generatedColors, _, err := aseprite.ApplyAutoShading(
				img,
				dir,
				0.5,
				"cell",
				true,
			)
			if err != nil {
				t.Fatalf("Auto-shading failed for direction %s: %v", dir, err)
			}

			if len(generatedColors) == 0 {
				t.Errorf("No colors generated for direction %s", dir)
			}

			t.Logf("✓ Shading from %s: %d colors", dir, len(generatedColors))
		})
	}
}

func TestIntegration_ApplyAutoShading_WithHueShift(t *testing.T) {
	// Create test image
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 8; y < 24; y++ {
		for x := 8; x < 24; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, G: 128, B: 0, A: 255})
		}
	}

	// Test with hue shift enabled
	_, colorsWithShift, _, err := aseprite.ApplyAutoShading(img, "top_left", 0.6, "cell", true)
	if err != nil {
		t.Fatalf("Auto-shading with hue shift failed: %v", err)
	}

	// Test with hue shift disabled
	_, colorsWithoutShift, _, err := aseprite.ApplyAutoShading(img, "top_left", 0.6, "cell", false)
	if err != nil {
		t.Fatalf("Auto-shading without hue shift failed: %v", err)
	}

	if len(colorsWithShift) == 0 || len(colorsWithoutShift) == 0 {
		t.Fatal("Generated colors should not be empty")
	}

	t.Logf("✓ With hue shift: %d colors", len(colorsWithShift))
	t.Logf("✓ Without hue shift: %d colors", len(colorsWithoutShift))
}

func TestIntegration_ApplyAutoShading_IndexedPreservesColorFamilies(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-autoshading-indexed-families.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(24, 12, aseprite.ColorModeIndexed, spritePath), ""); err != nil {
		t.Fatalf("create indexed canvas: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.SetPalette([]string{
		"#7A3E1DFF", // brown region
		"#2E8B57FF", // green region
		"#00000000", // transparent index
	}), spritePath); err != nil {
		t.Fatalf("set indexed palette: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, `local spr = app.activeSprite
spr.transparentColor = 2
spr.layers[1]:cel(1).image:clear(2)
spr:saveAs(spr.filename)`, spritePath); err != nil {
		t.Fatalf("set transparent index: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.DrawRectangle("Layer 1", 1, 0, 0, 10, 12, aseprite.Color{R: 122, G: 62, B: 29, A: 255}, true, true), spritePath); err != nil {
		t.Fatalf("draw brown region: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.DrawRectangle("Layer 1", 1, 14, 0, 10, 12, aseprite.Color{R: 46, G: 139, B: 87, A: 255}, true, true), spritePath); err != nil {
		t.Fatalf("draw green region: %v", err)
	}

	originalPNG := testutil.TempSpritePath(t, "test-autoshading-indexed-source.png")
	exportCel := fmt.Sprintf(`local cel = app.activeSprite.layers[1]:cel(1)
cel.image:saveAs(%q)`, originalPNG)
	if _, err := client.ExecuteLua(ctx, exportCel, spritePath); err != nil {
		t.Fatalf("export indexed cel: %v", err)
	}
	sourceFile, err := os.Open(originalPNG)
	if err != nil {
		t.Fatalf("open indexed cel PNG: %v", err)
	}
	sourceImage, err := png.Decode(sourceFile)
	_ = sourceFile.Close()
	if err != nil {
		t.Fatalf("decode indexed cel PNG: %v", err)
	}

	shadedImage, generatedColors, regionsShaded, err := aseprite.ApplyAutoShading(sourceImage, "top_left", 0.5, "smooth", true)
	if err != nil {
		t.Fatalf("shade indexed source: %v", err)
	}
	shadedPNG := testutil.TempSpritePath(t, "test-autoshading-indexed-result.png")
	shadedFile, err := os.Create(shadedPNG)
	if err != nil {
		t.Fatalf("create shaded PNG: %v", err)
	}
	if err := png.Encode(shadedFile, shadedImage); err != nil {
		_ = shadedFile.Close()
		t.Fatalf("encode shaded PNG: %v", err)
	}
	if err := shadedFile.Close(); err != nil {
		t.Fatalf("close shaded PNG: %v", err)
	}

	output, err := client.ExecuteLua(ctx, gen.ApplyAutoShadingResult(shadedPNG, "Layer 1", 1, generatedColors, regionsShaded), spritePath)
	if err != nil {
		t.Fatalf("apply indexed shading result: %v", err)
	}
	var result ApplyAutoShadingOutput
	if err := parseJSON(output, &result); err != nil {
		t.Fatalf("parse indexed shading result: %v", err)
	}
	if len(result.Palette) < 3 || result.Palette[0] != "#7A3E1D" || result.Palette[1] != "#2E8B57" || result.Palette[2] != "#000000" {
		t.Fatalf("original palette entries changed: %v", result.Palette)
	}
	if result.ColorsAdded <= 0 || result.ColorsAdded != len(result.Palette)-3 {
		t.Fatalf("colors_added = %d, palette size = %d; want only distinct appended colors", result.ColorsAdded, len(result.Palette))
	}

	pixelOutput, err := client.ExecuteLua(ctx, gen.GetPixels("Layer 1", 1, 0, 0, 24, 12), spritePath)
	if err != nil {
		t.Fatalf("get shaded indexed pixels: %v", err)
	}
	pixels, err := testutil.ParsePixelData(pixelOutput)
	if err != nil {
		t.Fatalf("parse shaded indexed pixels: %v", err)
	}
	pixelAt := make(map[string]string, len(pixels))
	for _, pixel := range pixels {
		pixelAt[testutil.FormatPixelPos(pixel.X, pixel.Y)] = pixel.Color
	}
	assertDominantColor(t, pixelAt["5,6"], 'r')
	assertDominantColor(t, pixelAt["18,6"], 'g')
	if got := pixelAt["12,6"]; got != "#00000000" {
		t.Fatalf("transparent gap = %s, want #00000000", got)
	}

	exportedPNG := testutil.TempSpritePath(t, "test-autoshading-indexed-export.png")
	if _, err := client.ExecuteLua(ctx, gen.ExportSprite(exportedPNG, 1), spritePath); err != nil {
		t.Fatalf("export shaded indexed sprite: %v", err)
	}
	exportedFile, err := os.Open(exportedPNG)
	if err != nil {
		t.Fatalf("open shaded export: %v", err)
	}
	exportedImage, err := png.Decode(exportedFile)
	_ = exportedFile.Close()
	if err != nil {
		t.Fatalf("decode shaded export: %v", err)
	}
	assertDominantRGBA(t, exportedImage.At(5, 6), 'r')
	assertDominantRGBA(t, exportedImage.At(18, 6), 'g')
	_, _, _, alpha := exportedImage.At(12, 6).RGBA()
	if alpha != 0 {
		t.Fatalf("exported transparent gap alpha = %d, want 0", alpha)
	}
}

func TestIntegration_ApplyAutoShading_IndexedFullPaletteUsesExistingColors(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	palette := []string{"#7A3E1DFF", "#2E8B57FF"}
	for i := 2; i < 255; i++ {
		value := uint8(i)
		palette = append(palette, fmt.Sprintf("#%02X%02X%02XFF", value, value, value))
	}
	palette = append(palette, "#00000000")

	spritePath := testutil.TempSpritePath(t, "test-autoshading-indexed-full-palette.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(3, 1, aseprite.ColorModeIndexed, spritePath), ""); err != nil {
		t.Fatalf("create full-palette canvas: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, gen.SetPalette(palette), spritePath); err != nil {
		t.Fatalf("set full palette: %v", err)
	}
	if _, err := client.ExecuteLua(ctx, `local spr = app.activeSprite
spr.transparentColor = 255
spr.layers[1]:cel(1).image:clear(255)
spr:saveAs(spr.filename)`, spritePath); err != nil {
		t.Fatalf("initialize full-palette transparency: %v", err)
	}

	shaded := image.NewRGBA(image.Rect(0, 0, 3, 1))
	shaded.SetRGBA(0, 0, color.RGBA{R: 106, G: 50, B: 24, A: 255})
	shaded.SetRGBA(1, 0, color.RGBA{R: 54, G: 160, B: 102, A: 255})
	shaded.SetRGBA(2, 0, color.RGBA{})
	shadedPNG := testutil.TempSpritePath(t, "test-autoshading-indexed-full-palette.png")
	file, err := os.Create(shadedPNG)
	if err != nil {
		t.Fatalf("create full-palette shaded PNG: %v", err)
	}
	if err := png.Encode(file, shaded); err != nil {
		_ = file.Close()
		t.Fatalf("encode full-palette shaded PNG: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close full-palette shaded PNG: %v", err)
	}

	output, err := client.ExecuteLua(ctx, gen.ApplyAutoShadingResult(
		shadedPNG,
		"Layer 1",
		1,
		[]string{"#1234AB", "#ABC123"},
		2,
	), spritePath)
	if err != nil {
		t.Fatalf("apply full-palette shading: %v", err)
	}
	var result ApplyAutoShadingOutput
	if err := parseJSON(output, &result); err != nil {
		t.Fatalf("parse full-palette result: %v", err)
	}
	if result.ColorsAdded != 0 {
		t.Fatalf("colors_added = %d, want 0 for a full palette", result.ColorsAdded)
	}
	if len(result.Palette) != 256 {
		t.Fatalf("palette size = %d, want 256", len(result.Palette))
	}
	if result.Palette[0] != "#7A3E1D" || result.Palette[1] != "#2E8B57" || result.Palette[255] != "#000000" {
		t.Fatalf("full palette entries changed: first=%v last=%s", result.Palette[:2], result.Palette[255])
	}

	pixelOutput, err := client.ExecuteLua(ctx, gen.GetPixels("Layer 1", 1, 0, 0, 3, 1), spritePath)
	if err != nil {
		t.Fatalf("get full-palette pixels: %v", err)
	}
	pixels, err := testutil.ParsePixelData(pixelOutput)
	if err != nil {
		t.Fatalf("parse full-palette pixels: %v", err)
	}
	if len(pixels) != 3 {
		t.Fatalf("pixel count = %d, want 3", len(pixels))
	}
	if pixels[0].Color != "#7A3E1DFF" || pixels[1].Color != "#2E8B57FF" {
		t.Fatalf("full-palette fallback changed original indices: %s, %s", pixels[0].Color, pixels[1].Color)
	}
	if pixels[2].Color != "#00000000" {
		t.Fatalf("full-palette transparent pixel = %s, want #00000000", pixels[2].Color)
	}
}

func TestIntegration_ApplyAutoShading_RGBApplyResult(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	client := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, 30*time.Second)
	gen := aseprite.NewLuaGenerator()
	ctx := context.Background()

	spritePath := testutil.TempSpritePath(t, "test-autoshading-rgb-result.aseprite")
	if _, err := client.ExecuteLua(ctx, gen.CreateCanvas(3, 1, aseprite.ColorModeRGB, spritePath), ""); err != nil {
		t.Fatalf("create RGB canvas: %v", err)
	}
	shaded := image.NewRGBA(image.Rect(0, 0, 3, 1))
	shaded.SetRGBA(0, 0, color.RGBA{R: 132, G: 66, B: 31, A: 255})
	shaded.SetRGBA(1, 0, color.RGBA{R: 57, G: 161, B: 105, A: 255})
	shaded.SetRGBA(2, 0, color.RGBA{})
	shadedPNG := testutil.TempSpritePath(t, "test-autoshading-rgb-result.png")
	file, err := os.Create(shadedPNG)
	if err != nil {
		t.Fatalf("create RGB shaded PNG: %v", err)
	}
	if err := png.Encode(file, shaded); err != nil {
		_ = file.Close()
		t.Fatalf("encode RGB shaded PNG: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close RGB shaded PNG: %v", err)
	}

	if _, err := client.ExecuteLua(ctx, gen.ApplyAutoShadingResult(
		shadedPNG,
		"Layer 1",
		1,
		[]string{"#84421F", "#39A169"},
		2,
	), spritePath); err != nil {
		t.Fatalf("apply RGB shading result: %v", err)
	}
	pixelOutput, err := client.ExecuteLua(ctx, gen.GetPixels("Layer 1", 1, 0, 0, 3, 1), spritePath)
	if err != nil {
		t.Fatalf("get RGB shaded pixels: %v", err)
	}
	pixels, err := testutil.ParsePixelData(pixelOutput)
	if err != nil {
		t.Fatalf("parse RGB shaded pixels: %v", err)
	}
	want := []string{"#84421FFF", "#39A169FF", "#00000000"}
	if len(pixels) != len(want) {
		t.Fatalf("RGB pixel count = %d, want %d", len(pixels), len(want))
	}
	for i, pixel := range pixels {
		if pixel.Color != want[i] {
			t.Fatalf("RGB pixel %d = %s, want %s", i, pixel.Color, want[i])
		}
	}
}

func assertDominantColor(t *testing.T, hexColor string, dominant byte) {
	t.Helper()
	var r, g, b, a uint8
	if _, err := fmt.Sscanf(hexColor, "#%02X%02X%02X%02X", &r, &g, &b, &a); err != nil {
		t.Fatalf("parse color %q: %v", hexColor, err)
	}
	assertDominantRGBA(t, color.RGBA{R: r, G: g, B: b, A: a}, dominant)
}

func assertDominantRGBA(t *testing.T, c color.Color, dominant byte) {
	t.Helper()
	r16, g16, b16, a16 := c.RGBA()
	r, g, b := uint8(r16>>8), uint8(g16>>8), uint8(b16>>8)
	if a16 == 0 {
		t.Fatal("expected opaque color, got transparent")
	}
	switch dominant {
	case 'r':
		if r <= g || r <= b {
			t.Fatalf("expected red-dominant color, got rgb(%d,%d,%d)", r, g, b)
		}
	case 'g':
		if g <= r || g <= b {
			t.Fatalf("expected green-dominant color, got rgb(%d,%d,%d)", r, g, b)
		}
	default:
		t.Fatalf("unsupported dominant channel %q", dominant)
	}
}

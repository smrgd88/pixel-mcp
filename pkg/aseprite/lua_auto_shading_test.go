package aseprite

import (
	"strings"
	"testing"
)

func TestApplyAutoShadingResult_MapsIndexedPixelsThroughPalette(t *testing.T) {
	gen := NewLuaGenerator()
	script := gen.ApplyAutoShadingResult(
		"/tmp/shaded.png",
		"Layer 1",
		1,
		[]string{"#663311", "#7A3E1D", "#8F5A38"},
		2,
	)

	required := []string{
		"if spr.colorMode == ColorMode.INDEXED then",
		"if i ~= spr.transparentColor",
		"return spr.transparentColor",
		"addGeneratedColor(color)",
		"paletteCapacityExhausted = true",
		"findNearestPaletteIndex(r, g, b, a, originalIndex)",
		"local originalIndex = cel.image:getPixel(x, y)",
		"local paletteIndexCache = {}",
		"finalImg:drawPixel(x, y, paletteIndex)",
		`"colors_added": %d`,
		"}]], colorsAdded,",
	}
	for _, fragment := range required {
		if !strings.Contains(script, fragment) {
			t.Errorf("script missing %q", fragment)
		}
	}

	paletteSetup := strings.Index(script, "for _, color in ipairs(generatedColors) do")
	pixelMapping := strings.Index(script, "for y = 0, shadedImg.height - 1 do")
	if paletteSetup < 0 || pixelMapping < 0 || paletteSetup >= pixelMapping {
		t.Error("generated palette colors must be installed before indexed pixel mapping")
	}
}

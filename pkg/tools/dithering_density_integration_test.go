//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func ditherDensityArgs(path, pattern string) map[string]any {
	return map[string]any{"sprite_path": path, "layer_name": "Layer 1", "frame_number": 1, "region": map[string]int{"x": 2, "y": 3, "width": 8, "height": 8}, "color1": "#FF0000", "color2": "#0000FF", "pattern": pattern}
}

func assertDitherCounts(f *behaviorFixture, path string, red, blue int) {
	f.t.Helper()
	f.lua(path, fmt.Sprintf(`local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1)
local red,blue=0,0
for y=0,15 do for x=0,15 do local v=im:getPixel(x,y)
 if x>=2 and x<10 and y>=3 and y<11 then
  assert(app.pixelColor.rgbaA(v)==255,"unexpected alpha")
  if v==app.pixelColor.rgba(255,0,0,255) then red=red+1 elseif v==app.pixelColor.rgba(0,0,255,255) then blue=blue+1 else error("unexpected color") end
 else assert(app.pixelColor.rgbaA(v)==0,"pixel outside region changed") end
end end
assert(red==%d and blue==%d,string.format("red=%%d blue=%%d",red,blue))`, red, blue))
}

func TestDitherDensityDefaultsAndBounds(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	for _, tc := range []struct {
		name      string
		density   any
		red, blue int
	}{
		{"omitted", nil, 32, 32}, {"zero", 0.0, 64, 0}, {"half", 0.5, 32, 32}, {"one", 1.0, 0, 64},
		{"quarter", 0.25, 48, 16}, {"three quarters", 0.75, 16, 48}, {"small positive", 0.0000001, 48, 16},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := ditherDensityArgs(p, "bayer_2x2")
			if tc.density != nil {
				args["density"] = tc.density
			}
			f.call("draw_with_dither", args)
			assertDitherCounts(f, p, tc.red, tc.blue)
		})
	}
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	for _, density := range []any{-0.01, 1.01, "0", false} {
		args := ditherDensityArgs(p, "bayer_2x2")
		args["density"] = density
		r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "draw_with_dither", Arguments: args})
		if err != nil {
			require.Contains(t, err.Error(), "invalid params")
		} else {
			require.True(t, r.IsError)
		}
		after, err := os.ReadFile(p)
		require.NoError(t, err)
		require.Equal(t, before, after, "invalid density changed source")
	}
}

func TestDitherDensityEndpointsAllPatterns(t *testing.T) {
	for _, pattern := range []string{"bayer_2x2", "bayer_4x4", "bayer_8x8", "checkerboard", "grass", "water", "stone", "cloud", "brick", "dots", "diagonal", "cross", "noise", "horizontal_lines", "vertical_lines", "floyd_steinberg"} {
		t.Run(pattern, func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			args := ditherDensityArgs(p, pattern)
			args["density"] = 0.0
			f.call("draw_with_dither", args)
			assertDitherCounts(f, p, 64, 0)
			args["density"] = 1.0
			f.call("draw_with_dither", args)
			assertDitherCounts(f, p, 0, 64)
		})
	}
}

func TestDitherDensityIndexedEndpoints(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeIndexed)
	f.lua(p, `local s=app.activeSprite;local pal=s.palettes[1];pal:resize(3);pal:setColor(0,Color{r=0,g=0,b=0,a=0});pal:setColor(1,Color{r=255,g=0,b=0,a=255});pal:setColor(2,Color{r=0,g=0,b=255,a=255});s.transparentColor=0;local im=Image(s.spec);im:clear(0);s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
	for _, pattern := range []string{"bayer_2x2", "floyd_steinberg"} {
		args := ditherDensityArgs(p, pattern)
		args["density"] = 0.0
		f.call("draw_with_dither", args)
		assertDitherCounts(f, p, 64, 0)
		args["density"] = 1.0
		f.call("draw_with_dither", args)
		assertDitherCounts(f, p, 0, 64)
	}
}

func TestDitherDensitySchemaAndFloydInterior(t *testing.T) {
	f := newBehaviorFixture(t)
	list, err := f.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	found := false
	for _, tool := range list.Tools {
		if tool.Name == "draw_with_dither" {
			found = true
			encoded, err := json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			var schema struct {
				Required   []string                   `json:"required"`
				Properties map[string]json.RawMessage `json:"properties"`
			}
			require.NoError(t, json.Unmarshal(encoded, &schema))
			require.NotContains(t, schema.Required, "density")
			var densitySchema struct {
				Type []string `json:"type"`
			}
			require.NoError(t, json.Unmarshal(schema.Properties["density"], &densitySchema))
			require.ElementsMatch(t, []string{"number", "null"}, densitySchema.Type)
		}
	}
	require.True(t, found)
	p := f.sprite(aseprite.ColorModeRGB)
	nullArgs := ditherDensityArgs(p, "bayer_2x2")
	nullArgs["density"] = nil
	f.call("draw_with_dither", nullArgs)
	assertDitherCounts(f, p, 32, 32)
	var baseline string
	for _, density := range []any{nil, 0.5, 0.25, 0.75} {
		args := ditherDensityArgs(p, "floyd_steinberg")
		if density != nil {
			args["density"] = density
		}
		f.call("draw_with_dither", args)
		pixels := f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);local values={};local red,blue=0,0;for y=3,10 do for x=2,9 do local v=im:getPixel(x,y);table.insert(values,v);if app.pixelColor.rgbaR(v)==255 then red=red+1 else blue=blue+1 end end end;assert(red>0 and blue>0);print(json.encode(values))`)
		if baseline == "" {
			baseline = pixels
		} else {
			require.Equal(t, baseline, pixels, "Floyd interior behavior changed")
		}
	}
}

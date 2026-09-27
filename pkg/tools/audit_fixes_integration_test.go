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
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func auditFixture(t *testing.T) *behaviorFixture {
	f := newBehaviorFixture(t)
	// A separate server registers the additional audited consumers without altering shared fixtures.
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	server := mcp.NewServer(&mcp.Implementation{Name: "audit", Version: "1"}, nil)
	RegisterPaletteTools(server, f.client, f.gen, cfg, logger)
	RegisterAnalysisTools(server, f.client, f.gen, cfg, logger)
	RegisterAntialiasingTools(server, f.client, f.gen, cfg, logger)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "audit-client", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &behaviorFixture{t, cs, f.client, f.gen}
}

func TestAuditPaletteMaskResize(t *testing.T) {
	for _, oldMask := range []int{0, 3, 255} {
		t.Run(fmt.Sprint(oldMask), func(t *testing.T) {
			f := auditFixture(t)
			p := f.sprite(aseprite.ColorModeIndexed)
			f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local pal=s.palettes[1];pal:resize(4)
pal:setColor(0,Color{r=255,g=0,b=0,a=255});pal:setColor(1,Color{r=0,g=0,b=255,a=255});pal:setColor(2,Color{r=0,g=255,b=0,a=255});pal:setColor(3,Color{r=255,g=255,b=255,a=255});s.transparentColor=%d
local im=Image(s.spec);for y=0,15 do for x=0,15 do im:drawPixel(x,y,x<8 and 0 or 1) end end;im:drawPixel(0,0,s.transparentColor);s:newCel(s.layers[1],1,im,Point(0,0));s:saveAs(s.filename)`, oldMask))
			snapshot := f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);local v={};for px in im:pixels() do v[#v+1]=px() end;print(json.encode(v))`)
			f.call("set_palette", map[string]any{"sprite_path": p, "colors": []string{"#FF0000", "#0000FF"}})
			f.lua(p, fmt.Sprintf(`assert(app.activeSprite.transparentColor==%d,"mask changed")`, oldMask))
			after := f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);local v={};for px in im:pixels() do v[#v+1]=px() end;print(json.encode(v))`)
			require.Equal(t, snapshot, after)
			f.call("add_palette_color", map[string]any{"sprite_path": p, "color": "#00FF00"})
			f.lua(p, fmt.Sprintf(`assert(app.activeSprite.transparentColor==%d,"growth changed mask")`, oldMask))
		})
	}
}

func TestAuditReferenceZeroThreshold(t *testing.T) {
	f := auditFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	png := t.TempDir() + "/low.png"
	f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);for y=0,15 do for x=0,15 do local v=x<8 and 100 or 101;im:drawPixel(x,y,app.pixelColor.rgba(v,v,v,255)) end end;s:newCel(s.layers[1],1,im);s:saveAs(s.filename);im:saveAs(%q)`, png))
	var grids []any
	for _, threshold := range []any{nil, 0, 1, 30} {
		a := map[string]any{"reference_path": png, "target_width": 16, "target_height": 16}
		if threshold != nil {
			a["edge_threshold"] = threshold
		}
		r := f.call("analyze_reference", a)
		grids = append(grids, r["edge_map"])
	}
	require.Equal(t, grids[1], grids[2], "zero must retain low-contrast edges")
	require.NotEqual(t, grids[1], grids[3])
	require.Equal(t, grids[0], grids[3])
}

func TestAuditAAThreshold(t *testing.T) {
	f := auditFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);for y=1,6 do for x=y,y+1 do im:drawPixel(x,y,app.pixelColor.rgba(255,255,255,64)) end end;s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	totals := map[int]float64{}
	for _, threshold := range []int{0, 1, 63, 64, 128, 255} {
		r := f.call("suggest_antialiasing", map[string]any{"sprite_path": p, "layer_name": "Layer 1", "frame_number": 1, "threshold": threshold, "auto_apply": false})
		totals[threshold] = r["total_edges"].(float64)
	}
	require.Positive(t, totals[0])
	require.Equal(t, totals[0], totals[63])
	require.Zero(t, totals[64])
	require.Zero(t, totals[128])
	require.Zero(t, totals[255])
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestAuditDitherInterior(t *testing.T) {
	for _, pattern := range []string{"floyd_steinberg", "checkerboard", "grass", "water", "stone", "cloud", "brick", "dots", "diagonal", "cross", "noise", "horizontal_lines", "vertical_lines"} {
		t.Run(pattern, func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			var counts []int
			for _, density := range []float64{.25, .5, .75} {
				a := ditherDensityArgs(p, pattern)
				a["density"] = density
				f.call("draw_with_dither", a)
				raw := f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);local blue=0;for y=3,10 do for x=2,9 do local v=im:getPixel(x,y);if app.pixelColor.rgbaB(v)==255 then blue=blue+1 end end end;print(blue)`)
				var n int
				_, err := fmt.Sscanf(raw, "%d", &n)
				require.NoError(t, err)
				counts = append(counts, n)
			}
			t.Logf("density .25/.5/.75 color2 counts: %v", counts)
			require.Less(t, counts[0], counts[1])
			require.Less(t, counts[1], counts[2])
		})
	}
}

func TestAuditThresholdSchema(t *testing.T) {
	f := auditFixture(t)
	list, err := f.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	for _, tool := range list.Tools {
		field := ""
		if tool.Name == "analyze_reference" {
			field = "edge_threshold"
		}
		if tool.Name == "suggest_antialiasing" {
			field = "threshold"
		}
		if field == "" {
			continue
		}
		b, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		var schema map[string]any
		require.NoError(t, json.Unmarshal(b, &schema))
		prop := schema["properties"].(map[string]any)[field].(map[string]any)
		require.ElementsMatch(t, []any{"integer", "null"}, prop["type"])
	}
}

func TestAuditThresholdDefaultsErrorsAndApply(t *testing.T) {
	f := auditFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);for y=1,6 do for x=y,y+1 do im:drawPixel(x,y,app.pixelColor.rgba(255,255,255,64)) end end;s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
	args := map[string]any{"sprite_path": p, "layer_name": "Layer 1", "frame_number": 1, "auto_apply": true}
	baseline := f.call("suggest_antialiasing", args)
	require.Equal(t, float64(0), baseline["total_edges"])
	require.Equal(t, false, baseline["applied"])
	args["threshold"] = nil
	require.Equal(t, baseline, f.call("suggest_antialiasing", args))
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	for _, value := range []any{-1, 256, "128", 1.5} {
		args["threshold"] = value
		r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suggest_antialiasing", Arguments: args})
		if err != nil {
			require.Contains(t, err.Error(), "invalid params")
		} else {
			require.True(t, r.IsError)
		}
		after, err := os.ReadFile(p)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
	args["threshold"] = 0
	r := f.call("suggest_antialiasing", args)
	require.Positive(t, r["total_edges"].(float64))
	require.Equal(t, true, r["applied"])
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
}

func TestAuditDitherThinAndStable(t *testing.T) {
	for _, shape := range [][2]int{{1, 8}, {8, 1}, {1, 1}, {3, 5}} {
		for _, pattern := range []string{"floyd_steinberg", "checkerboard", "dots"} {
			t.Run(fmt.Sprintf("%s/%dx%d", pattern, shape[0], shape[1]), func(t *testing.T) {
				f := newBehaviorFixture(t)
				p := f.sprite(aseprite.ColorModeRGB)
				args := ditherDensityArgs(p, pattern)
				args["region"] = map[string]int{"x": 2, "y": 3, "width": shape[0], "height": shape[1]}
				prior := -1
				for _, density := range []float64{0, .25, .5, .75, 1} {
					args["density"] = density
					f.call("draw_with_dither", args)
					code := fmt.Sprintf(`local s=app.activeSprite;local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,1);local b=0;local values={};for y=0,15 do for x=0,15 do local v=im:getPixel(x,y);if x>=2 and x<2+%d and y>=3 and y<3+%d then assert(app.pixelColor.rgbaA(v)==255);if app.pixelColor.rgbaB(v)==255 then b=b+1 end;values[#values+1]=v else assert(app.pixelColor.rgbaA(v)==0) end end end;print(json.encode({blue=b,pixels=values}))`, shape[0], shape[1])
					first := f.lua(p, code)
					f.call("draw_with_dither", args)
					require.Equal(t, first, f.lua(p, code), "repeat must be deterministic")
					var state struct {
						Blue int `json:"blue"`
					}
					require.NoError(t, json.Unmarshal([]byte(first), &state))
					require.GreaterOrEqual(t, state.Blue, prior)
					prior = state.Blue
					if density == 0 {
						require.Zero(t, state.Blue)
					}
					if density == 1 {
						require.Equal(t, shape[0]*shape[1], state.Blue)
					}
				}
			})
		}
	}
}

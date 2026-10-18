//go:build integration

package tools

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Observe native Image userdata through weak references without disabling GC or
// retaining images ourselves. A small fixture detects accumulation before it can
// become an OOM; the same check runs through the real generated export script.
func TestExportBudgetScratchRetention(t *testing.T) {
	for _, mode := range []struct {
		name, lua, pixel string
		want             color.NRGBA
	}{
		{"rgb", "ColorMode.RGB", "app.pixelColor.rgba(255,0,0,255)", color.NRGBA{255, 0, 0, 255}},
		{"gray", "ColorMode.GRAY", "app.pixelColor.graya(127,255)", color.NRGBA{127, 127, 127, 255}},
		{"indexed", "ColorMode.INDEXED", "1", color.NRGBA{255, 0, 0, 255}},
	} {
		for _, n := range []int{8, 80} {
			for _, empty := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/empty=%t", mode.name, n, empty), func(t *testing.T) {
					f := newExportOptionsFixture(t)
					path := filepath.Join(t.TempDir(), "source.aseprite")
					f.lua("", fmt.Sprintf(`local s=Sprite(512,512,%s)
if s.colorMode==ColorMode.INDEXED then
 s.transparentColor=3
 local pal=s.palettes[1];pal:resize(4);pal:setColor(1,Color{r=255,g=0,b=0,a=255})
 s.cels[1].image:clear(3)
end
for i=1,%d do
 if i>1 then s:newEmptyFrame() end
 s.frames[i].duration=i/100
end
if not %t then
 local im=Image(1,1,s.colorMode);im:clear(%s);s:newCel(s.layers[1],%d,im,Point(3,5))
end
s:saveAs("%s")`, mode.lua, n, empty, mode.pixel, n, aseprite.EscapeString(path)))
					before, err := os.ReadFile(path)
					require.NoError(t, err)
					stat, err := os.Stat(path)
					require.NoError(t, err)
					dir := t.TempDir()
					outputs := []string{filepath.Join(dir, "sheet.png"), filepath.Join(dir, "sheet.json")}
					for _, out := range outputs {
						require.NoError(t, os.WriteFile(out, []byte("original"), 0600))
					}
					ctx := context.Background()
					selection := aseprite.ExportSelection{}
					err = aseprite.WithFileLocks(ctx, append([]string{path}, outputs...), func(ctx context.Context) error {
						return aseprite.WithSpriteAccess(ctx, path, false, func(ctx context.Context) error {
							plan, e := planExport(ctx, f.client, f.gen, path, "", selection)
							if e != nil {
								return e
							}
							allocation, e := measureSheet(ctx, f.client, f.gen, path, plan, selection, "packed", 0, 0, 0, true, false)
							if e != nil {
								return e
							}
							return withExportOutputFiles(ctx, outputs, nil, func(staged []string) error {
								script := `local originalImage=Image
local tracked=setmetatable({}, {__mode="v"})
local serial=0
Image=function(...)
 local im=originalImage(...)
 if im.width==512 and im.height==512 then
  serial=serial+1;tracked[serial]=im
  local live=0
  for _,value in pairs(tracked) do live=live+1 end
  assert(live<=2,"canvas scratch retention exceeded")
 end
 return im
end
` + f.gen.ExportPlannedSheet(staged[0], staged[1], "packed", 0, 0, 0, true, false, selection, allocation.Samples)
								raw, e := f.client.ExecuteLua(ctx, script, path)
								if e != nil {
									return e
								}
								size, e := decodeExportSheetResult(raw)
								if e != nil {
									return e
								}
								if e = validateSheetMetadata(staged[1], "sheet.png", plan, size.Width, size.Height, allocation); e != nil {
									return e
								}
								return verifyExportRevision(ctx, path, plan.Revision)
							})
						})
					})
					sourceUnchanged(t, path, before, stat)
					stages, e := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
					require.NoError(t, e)
					require.Empty(t, stages)
					if empty {
						require.ErrorContains(t, err, "Export selection is empty")
						require.False(t, strings.Contains(err.Error(), "scratch retention"))
						for _, out := range outputs {
							data, e := os.ReadFile(out)
							require.NoError(t, e)
							require.Equal(t, "original", string(data))
						}
						return
					}
					require.NoError(t, err)
					im := decodeExportPNG(t, outputs[0])
					require.Equal(t, image.Rect(0, 0, 2, 1), im.Bounds())
					data := readSheet(t, outputs[1])
					require.Len(t, data.Frames, n)
					for _, fr := range data.Frames {
						require.Equal(t, fr.Number*10, fr.Duration)
						require.Equal(t, 1, fr.Frame.W)
						require.Equal(t, 1, fr.Frame.H)
						if fr.Number == n {
							require.Equal(t, sheetRect{3, 5, 1, 1}, fr.Offset)
							require.Equal(t, mode.want, color.NRGBAModel.Convert(im.At(fr.Frame.X, fr.Frame.Y)))
						} else {
							require.Equal(t, sheetRect{0, 0, 1, 1}, fr.Offset)
							_, _, _, a := im.At(fr.Frame.X, fr.Frame.Y).RGBA()
							require.Zero(t, a)
						}
					}
				})
			}
		}
	}
}

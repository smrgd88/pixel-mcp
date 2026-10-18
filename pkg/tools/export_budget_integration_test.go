//go:build integration

package tools

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestExportBudgetFormerStripRejection(t *testing.T) {
	f := newExportOptionsFixture(t)
	caps, err := f.client.GetCapabilities(context.Background())
	require.NoError(t, err)
	p := filepath.Join(t.TempDir(), "small.aseprite")
	f.lua("", fmt.Sprintf(`local s=Sprite(128,128)
for i=1,65 do
 if i>1 then s:newEmptyFrame() end
 local im=Image(128,128);im:clear(app.pixelColor.rgba(i,0,0,255))
 s:newCel(s.layers[1],i,im)
end
s:saveAs("%s")`, aseprite.EscapeString(p)))
	for _, layout := range []string{"rows", "columns", "packed"} {
		t.Run(layout, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "sheet.png")
			_, err := exportSpritesheet(context.Background(), f.client, f.gen, ExportSpritesheetInput{SpritePath: p, OutputPath: dest, Layout: layout})

			if layout == "packed" && !aseprite.HasStableSheetPacking(caps.Version) {
				require.ErrorContains(t, err, "budget")
				return
			}
			require.NoError(t, err)
			im := decodeExportPNG(t, dest)
			if layout == "rows" {
				require.Equal(t, image.Rect(0, 0, 8320, 128), im.Bounds())
			}
			if layout == "columns" {
				require.Equal(t, image.Rect(0, 0, 128, 8320), im.Bounds())
			}
			require.Less(t, im.Bounds().Dx()*im.Bounds().Dy(), 2*1024*1024)
			require.Len(t, readSheet(t, spritesheetDataPath(dest)).Frames, 65)
		})
	}
}

// The oracle is the unplanned native exporter, with real image decoding and
// native JSON; it does not use the allocation implementation to derive sizes.
func TestExportBudgetNativeOracle(t *testing.T) {
	f := newExportOptionsFixture(t)
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		p := optionsSprite(t, f, mode)
		f.lua(p, `local s=app.activeSprite
local layer=s.layers[1].layers[1]
for i=1,12 do
 if i>#s.frames then s:newEmptyFrame() end
 if layer:cel(i) then s:deleteCel(layer,i) end
 if i%4~=0 then
  local w,h=(i*7)%13+1,(i*3)%11+1
  if i==2 then w=8;h=4 end
  local im=Image(w,h,s.colorMode)
  if s.colorMode==ColorMode.INDEXED then im:clear(0)
  elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255))
  else im:clear(app.pixelColor.rgba(128,128,128,255)) end
  s:newCel(layer,i,im,Point(i%3,i%5))
 end
 s.frames[i].duration=i/100
end
s:saveAs(s.filename)`)
		for _, layout := range []string{"horizontal", "vertical", "rows", "columns", "packed"} {
			for _, trim := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/%t", mode, layout, trim), func(t *testing.T) {
					sel := aseprite.ExportSelection{First: 2, Last: 12, Selected: true}
					ctx := context.Background()
					plan, err := planExport(ctx, f.client, f.gen, p, "", sel)
					require.NoError(t, err)
					alloc, err := measureSheet(ctx, f.client, f.gen, p, plan, sel, layout, 2, 3, 1, trim, true)
					require.NoError(t, err)
					path := filepath.Join(t.TempDir(), "oracle.png")
					// ExecuteLua normally stages writes; bind this call explicitly read-only.
					err = aseprite.WithSpriteAccess(ctx, p, false, func(ctx context.Context) error {
						_, err := f.client.ExecuteLua(ctx, f.gen.ExportSelectedSheet(path, spritesheetDataPath(path), layout, 2, 3, 1, trim, true, sel), p)
						return err
					})
					require.NoError(t, err)
					im := decodeExportPNG(t, path)
					if alloc.Conservative {
						require.LessOrEqual(t, im.Bounds().Dx(), alloc.Width)
						require.LessOrEqual(t, im.Bounds().Dy(), alloc.Height)
					} else {
						require.Equal(t, image.Rect(0, 0, alloc.Width, alloc.Height), im.Bounds())
					}
					require.NoError(t, validateSheetMetadata(spritesheetDataPath(path), "oracle.png", plan, im.Bounds().Dx(), im.Bounds().Dy(), alloc))
					d := readSheet(t, spritesheetDataPath(path))
					for _, fr := range d.Frames {
						require.Equal(t, fr.Number*10, fr.Duration)
						require.GreaterOrEqual(t, fr.Number, 2)
						// Every nonempty source is gray 128; inspect center content, not padding.
						if fr.Number%4 != 0 {
							x, y := fr.Frame.X+1, fr.Frame.Y+1
							if !trim {
								x += fr.Number % 3
								y += fr.Number % 5
							}
							r, g, b, a := im.At(x, y).RGBA()
							require.Equal(t, uint32(0x8080), r)
							require.Equal(t, r, g)
							require.Equal(t, r, b)
							require.Equal(t, uint32(0xffff), a)
						}
					}
				})
			}
		}
	}
}

func TestExportBudgetTrimAndFormats(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := filepath.Join(t.TempDir(), "wide.aseprite")
	f.lua("", fmt.Sprintf(`local s=Sprite(32767,1)
for i=1,2 do
 if i>1 then s:newEmptyFrame() end
 local im=Image(1,1);im:clear(app.pixelColor.rgba(i*100,0,0,255));s:newCel(s.layers[1],i,im,Point(i,0))
end
s:saveAs("%s")`, aseprite.EscapeString(p)))
	for _, layout := range []string{"horizontal", "vertical", "rows", "columns", "packed"} {
		for _, ext := range []string{"png", "jpg", "gif", "bmp"} {
			t.Run(layout+"/"+ext, func(t *testing.T) {
				dest := filepath.Join(t.TempDir(), "sheet."+ext)
				_, err := exportSpritesheet(context.Background(), f.client, f.gen, ExportSpritesheetInput{SpritePath: p, OutputPath: dest, Layout: layout, Trim: true})
				require.NoError(t, err)
				d := readSheet(t, spritesheetDataPath(dest))
				require.Len(t, d.Frames, 2)
				require.Equal(t, 2, d.Meta.Size.W*d.Meta.Size.H)
				for _, fr := range d.Frames {
					require.Equal(t, sheetRect{fr.Number, 0, 1, 1}, fr.Offset)
				}
			})
		}
	}
}

func TestExportBudgetChangedPlanBeforeNativeAllocation(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeRGB)
	ctx := context.Background()
	sel := aseprite.ExportSelection{}
	plan, err := planExport(ctx, f.client, f.gen, p, "", sel)
	require.NoError(t, err)
	alloc, err := measureSheet(ctx, f.client, f.gen, p, plan, sel, "packed", 0, 0, 0, true, false)
	require.NoError(t, err)
	f.lua(p, `local s=app.activeSprite;local im=Image(12,12);im:clear(app.pixelColor.rgba(255,0,0,255));s:newCel(s.layers[1].layers[1],1,im);s:saveAs(s.filename)`)
	dest := filepath.Join(t.TempDir(), "must-not-exist.png")
	err = aseprite.WithSpriteAccess(ctx, p, false, func(ctx context.Context) error {
		raw, err := f.client.ExecuteLua(ctx, f.gen.ExportPlannedSheet(dest, spritesheetDataPath(dest), "packed", 0, 0, 0, true, false, sel, alloc.Samples), p)
		if err != nil {
			return err
		}
		_, err = decodeExportSheetResult(raw)
		return err
	})
	require.Error(t, err)
	require.Equal(t, "file_changed", diagnostics.Classify(err).Code)
	_, err = os.Stat(dest)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(spritesheetDataPath(dest))
	require.True(t, os.IsNotExist(err))
}

func TestExportBudgetNativeEqualAreaOrder(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := filepath.Join(t.TempDir(), "ties.aseprite")
	f.lua("", fmt.Sprintf(`local s=Sprite(16,16)
local sizes={{2,6},{3,4},{4,3},{6,2}}
for i=1,24 do
 if i>1 then s:newEmptyFrame() end
 local z=sizes[(i-1)%%4+1];local im=Image(z[1],z[2]);im:clear(app.pixelColor.rgba(i,0,0,255));s:newCel(s.layers[1],i,im,Point(1,1))
end
s:saveAs("%s")`, aseprite.EscapeString(p)))
	path := filepath.Join(t.TempDir(), "ties.png")
	_, err := exportSpritesheet(context.Background(), f.client, f.gen, ExportSpritesheetInput{SpritePath: p, OutputPath: path, Layout: "packed", Trim: true})
	require.NoError(t, err)
	im := decodeExportPNG(t, path)
	d := readSheet(t, spritesheetDataPath(path))
	require.Len(t, d.Frames, 24)
	for _, fr := range d.Frames {
		r, g, b, a := im.At(fr.Frame.X, fr.Frame.Y).RGBA()
		require.Equal(t, uint32(fr.Number*257), r)
		require.Zero(t, g)
		require.Zero(t, b)
		require.Equal(t, uint32(65535), a)
	}
}

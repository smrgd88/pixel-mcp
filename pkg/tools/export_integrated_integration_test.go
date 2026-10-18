//go:build integration

package tools

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Verify a single revision/history chain across all four merged feature areas.
// The old structural ID is deliberately reused by a different, same-named layer.
func TestExportIntegratedLayerTagUnlinkAndUndo(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f := newExportOptionsFixture(t)
			p := optionsSprite(t, f, mode)
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			initial := readStructure(t, f, map[string]any{"sprite_path": p})
			require.Equal(t, initial.Revision, readTags(t, f, p).Revision)
			f.call("create_layer_group", layerArgs(t, f, p, map[string]any{"name": "same/\"\n", "parent_id": "", "stack_index": 1}))
			f.call("move_layer", layerArgs(t, f, p, map[string]any{"layer_id": "2/2", "parent_id": "1", "stack_index": 1}))
			stale := map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "stale.png"), "format": "png", "frame_number": 0, "layer_id": "1/1", "expected_revision": initial.Revision, "include_hidden": true}
			rejectExport(t, f, "export_sprite", p, stale, "stale sprite revision")
			moved := f.call("move_layer", layerArgs(t, f, p, map[string]any{"layer_id": "2/1", "parent_id": "1", "stack_index": 2}))
			require.Equal(t, "1/2", moved["layer"].(map[string]any)["layer_id"])
			f.call("set_layer_properties", layerArgs(t, f, p, map[string]any{"layer_id": "1/2", "visible": false}))
			state := readStructure(t, f, map[string]any{"sprite_path": p})
			f.call("unlink_cel", map[string]any{"sprite_path": p, "layer_id": "1/2", "frame_number": 2, "expected_revision": state.Revision})
			state = readStructure(t, f, map[string]any{"sprite_path": p})
			f.call("set_cel_properties", map[string]any{"sprite_path": p, "layer_id": "1/2", "frame_number": 2, "expected_revision": state.Revision, "x": 0})
			tags := readTags(t, f, p)
			require.Len(t, tags.Tags, 1)
			f.call("set_tag_properties", map[string]any{"sprite_path": p, "tag_id": tags.Tags[0].TagID, "expected_revision": tags.Revision, "name": "integrated-walk", "from_frame": 1, "to_frame": 2, "direction": "pingpong_reverse", "repeats": 0})
			current := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "1/2"})
			require.Equal(t, current.Revision, readTags(t, f, p).Revision)
			require.Len(t, current.Layers, 1)
			cells := current.Layers[0].Cels
			require.Equal(t, 2, cells[0].LinkedCelCount)
			require.Equal(t, 1, cells[1].LinkedCelCount)
			require.Equal(t, cells[0].ImageRef, cells[2].ImageRef)
			require.Equal(t, 0, *cells[1].X)
			require.Equal(t, 3, *cells[0].X)
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			stat, err := os.Stat(p)
			require.NoError(t, err)
			ops := historyOperations(f, p)
			require.Len(t, ops, 7)
			args := map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "walk.png"), "format": "png", "frame_number": 0, "layer_id": "1/2", "expected_revision": current.Revision, "include_hidden": true, "tag": "integrated-walk"}
			out := f.call("export_sprite", args)
			files := out["files"].([]any)
			require.Len(t, files, 2)
			for i, entry := range files {
				file := entry.(map[string]any)
				require.Equal(t, float64(i+1), file["frame_number"])
				im := decodeExportPNG(t, file["path"].(string))
				require.Equal(t, 16, im.Bounds().Dx())
				require.Equal(t, 16, im.Bounds().Dy())
				x0 := 3
				if i == 1 {
					x0 = 0
				}
				for y := 0; y < 16; y++ {
					for x := 0; x < 16; x++ {
						want := color.RGBA{}
						if x >= x0 && x < x0+2 && y >= 5 && y < 8 {
							want = color.RGBA{128, 128, 128, 255}
						}
						require.Equal(t, want, color.RGBAModel.Convert(im.At(x, y)))
					}
				}
			}
			args["tag"] = "walk/\"\n"
			rejectExport(t, f, "export_sprite", p, args, "tag not found")
			sheet := filepath.Join(t.TempDir(), "sheet.png")
			sheetArgs := map[string]any{"sprite_path": p, "output_path": sheet, "layout": "horizontal", "padding": 0, "include_json": true, "layer_id": "1/2", "expected_revision": current.Revision, "include_hidden": true, "tag": "integrated-walk", "trim": true, "extrude": true, "border_padding": 1, "shape_padding": 2}
			f.call("export_spritesheet", sheetArgs)
			data := readSheet(t, spritesheetDataPath(sheet))
			require.Len(t, data.Frames, 2)
			for _, fr := range data.Frames {
				x := 3
				if fr.Number == 2 {
					x = 0
				}
				require.Equal(t, sheetRect{x, 5, 2, 3}, fr.Offset)
				require.Equal(t, 100*fr.Number, fr.Duration)
			}
			require.Len(t, data.Meta.Tags, 1)
			require.Equal(t, "integrated-walk", data.Meta.Tags[0].Name)
			require.Equal(t, 0, data.Meta.Tags[0].From)
			require.Equal(t, 1, data.Meta.Tags[0].To)
			require.Equal(t, "pingpong_reverse", data.Meta.Tags[0].Direction)
			textureBefore, err := os.ReadFile(sheet)
			require.NoError(t, err)
			jsonBefore, err := os.ReadFile(spritesheetDataPath(sheet))
			require.NoError(t, err)
			sheetArgs["expected_revision"] = tags.Revision
			rejectExport(t, f, "export_spritesheet", p, sheetArgs, "stale sprite revision")
			sheetArgs["expected_revision"] = current.Revision
			sheetArgs["include_hidden"] = false
			rejectExport(t, f, "export_spritesheet", p, sheetArgs, "selection is empty")
			sheetArgs["include_hidden"] = true
			sheetArgs["overwrite"] = false
			rejectExport(t, f, "export_spritesheet", p, sheetArgs, "overwrite=false")
			textureAfter, err := os.ReadFile(sheet)
			require.NoError(t, err)
			jsonAfter, err := os.ReadFile(spritesheetDataPath(sheet))
			require.NoError(t, err)
			require.Equal(t, textureBefore, textureAfter)
			require.Equal(t, jsonBefore, jsonAfter)
			sourceUnchanged(t, p, before, stat)
			require.Equal(t, ops, historyOperations(f, p))
			for _, op := range ops {
				f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op.(map[string]any)["operation_id"]})
			}
			restored, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, original, restored)
			require.Equal(t, initial.Revision, readStructure(t, f, map[string]any{"sprite_path": p}).Revision)
			// Byte-exact undo intentionally restores the original revision token.
			stale["output_path"] = filepath.Join(t.TempDir(), "restored.png")
			f.call("export_sprite", stale)
			f.lua(p, `local s=app.activeSprite;local l=s.layers[1].layers[1];assert(l:cel(1).image==l:cel(2).image and l:cel(1).image==l:cel(3).image);assert(l:cel(2).position==Point(3,5));assert(s.data=='keep' and s.properties('artist').text=='preserve')`)
		})
	}
}

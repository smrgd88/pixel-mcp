//go:build integration

package tools

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func newExportOptionsFixture(t *testing.T) *behaviorFixture {
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	cfg.EnableHistory = true
	cfg.SnapshotDir = filepath.Join(t.TempDir(), "history")
	c := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, cfg.Timeout)
	g := aseprite.NewLuaGenerator()
	l := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	s := mcp.NewServer(&mcp.Implementation{Name: "export-options", Version: "1"}, nil)
	RegisterExportTools(s, c, g, cfg, l)
	RegisterInspectionTools(s, c, g, cfg, l)
	RegisterCanvasTools(s, c, g, cfg, l)
	RegisterAnimationTools(s, c, g, cfg, l)
	RegisterHistoryTools(s, cfg, l)
	RegisterSnapshotTools(s, cfg, l)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &behaviorFixture{t, cs, c, g}
}

func optionsSprite(t *testing.T, f *behaviorFixture, mode aseprite.ColorMode) string {
	p := f.sprite(mode)
	f.lua(p, `local s=app.activeSprite
local a=s.layers[1];a.name='same/"\n'
local pal=s.palettes[1];pal:resize(4);pal:setColor(0,Color{r=128,g=128,b=128,a=255});pal:setColor(3,Color{r=255,g=0,b=255,a=255})
if s.colorMode==ColorMode.INDEXED then s.transparentColor=3 end
local function image(w,h,v)
 local im=Image(w,h,s.colorMode)
 if s.colorMode==ColorMode.INDEXED then im:clear(0)
 elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(v,255))
 else im:clear(app.pixelColor.rgba(v,v,v,255)) end
 return im
end
s:newCel(a,1,image(2,3,128),Point(3,5));s:newEmptyFrame();s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[2],s.frames[3]};app.command.LinkCels()
local group=s:newGroup();group.name=a.name;a.parent=group
local b=s:newLayer();b.name=a.name;b.parent=group;b.isVisible=false
for i=1,3 do s:newCel(b,i,image(1,1,255),Point(8,9)) end
local hidden=s:newLayer();hidden.name='hidden';hidden.isVisible=false
for i=1,3 do s:newCel(hidden,i,image(16,16,255)) end
local empty=s:newGroup();empty.name='empty'
local tag=s:newTag(2,3);tag.name='walk/"\n';tag.aniDir=AniDir.REVERSE
s.frames[1].duration=0.1;s.frames[2].duration=0.2;s.frames[3].duration=0.3
s.data='keep';a.data='paint metadata';a:cel(1).data='cel metadata';s.properties('artist').text='preserve'
s:saveAs(s.filename)`)
	return p
}

func decodeExportPNG(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	im, err := png.Decode(f)
	require.NoError(t, err)
	return im
}
func sourceUnchanged(t *testing.T, p string, before []byte, stat os.FileInfo) {
	t.Helper()
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	now, err := os.Stat(p)
	require.NoError(t, err)
	require.True(t, os.SameFile(stat, now))
	require.Equal(t, stat.Mode(), now.Mode())
	require.Equal(t, stat.ModTime(), now.ModTime())
}
func rejectExport(t *testing.T, f *behaviorFixture, tool, p string, args map[string]any, reason string) {
	t.Helper()
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	require.True(t, err != nil || r.IsError)
	if reason != "" {
		require.NoError(t, err)
		require.Contains(t, r.Content[0].(*mcp.TextContent).Text, reason)
	}
	sourceUnchanged(t, p, before, stat)
}

type sheetRect struct{ X, Y, W, H int }
type sheetFrame struct {
	Frame    sheetRect          `json:"frame"`
	Offset   sheetRect          `json:"spriteSourceSize"`
	Source   struct{ W, H int } `json:"sourceSize"`
	Duration int                `json:"duration"`
	Number   int                `json:"source_frame_number"`
	Trimmed  bool               `json:"trimmed"`
}
type sheetData struct {
	Frames map[string]sheetFrame `json:"frames"`
	Meta   struct {
		Image string             `json:"image"`
		Size  struct{ W, H int } `json:"size"`
		Tags  []struct {
			Name      string
			From, To  int
			Direction string
		} `json:"frameTags"`
	} `json:"meta"`
}

func readSheet(t *testing.T, p string) sheetData {
	t.Helper()
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	var d sheetData
	require.NoError(t, json.Unmarshal(b, &d))
	return d
}

func TestExportOptionsModesRangeAndSheetPixels(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f := newExportOptionsFixture(t)
			p := optionsSprite(t, f, mode)
			s := readStructure(t, f, map[string]any{"sprite_path": p})
			require.Equal(t, "1/1", s.Layers[1].LayerID)
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			stat, err := os.Stat(p)
			require.NoError(t, err)
			dest := filepath.Join(t.TempDir(), "walk007.png")
			args := map[string]any{"sprite_path": p, "output_path": dest, "format": "png", "frame_number": 0, "layer_id": "1/1", "expected_revision": s.Revision, "tag": "walk/\"\n", "trim": true}
			out := f.call("export_sprite", args)
			files := out["files"].([]any)
			require.Len(t, files, 2)
			for i, v := range files {
				file := v.(map[string]any)
				require.Equal(t, float64(i+2), file["frame_number"])
				require.Equal(t, filepath.Join(filepath.Dir(dest), fmt.Sprintf("walk007_%04d.png", i+2)), file["path"])
				im := decodeExportPNG(t, file["path"].(string))
				require.Equal(t, image.Rect(0, 0, 2, 3), im.Bounds())
				for y := 0; y < 3; y++ {
					for x := 0; x < 2; x++ {
						require.Equal(t, color.NRGBA{128, 128, 128, 255}, color.NRGBAModel.Convert(im.At(x, y)))
					}
				}
			}
			// Different name duplicates cannot redirect the structural selection. Group
			// selection retains the hidden child filter and native linked frame content.
			sheet := filepath.Join(t.TempDir(), "sheet.png")
			args = map[string]any{"sprite_path": p, "output_path": sheet, "layout": "horizontal", "padding": 0, "include_json": true, "layer_id": "1", "expected_revision": s.Revision, "frame_start": 2, "frame_end": 3, "trim": true, "extrude": true, "border_padding": 2, "shape_padding": 3, "inner_padding": 0}
			result := f.call("export_spritesheet", args)
			require.Equal(t, float64(2), result["frame_count"])
			require.Equal(t, spritesheetDataPath(sheet), result["metadata_path"])
			d := readSheet(t, spritesheetDataPath(sheet))
			im := decodeExportPNG(t, sheet)
			require.Len(t, d.Frames, 2)
			require.Equal(t, im.Bounds().Dx(), d.Meta.Size.W)
			require.Equal(t, "sheet.png", d.Meta.Image)
			require.Equal(t, image.Rect(0, 0, 15, 9), im.Bounds())
			expected := image.NewNRGBA(im.Bounds())
			for _, fr := range d.Frames {
				require.Contains(t, []int{2, 3}, fr.Number)
				require.Equal(t, fr.Number*100, fr.Duration)
				require.True(t, fr.Trimmed)
				require.Equal(t, sheetRect{3, 5, 2, 3}, fr.Offset)
				require.Equal(t, 2, fr.Frame.W)
				require.Equal(t, 3, fr.Frame.H)
				require.Equal(t, 3+(fr.Number-2)*7, fr.Frame.X)
				require.Equal(t, 3, fr.Frame.Y)
				for y := fr.Frame.Y - 1; y <= fr.Frame.Y+fr.Frame.H; y++ {
					for x := fr.Frame.X - 1; x <= fr.Frame.X+fr.Frame.W; x++ {
						expected.SetNRGBA(x, y, color.NRGBA{128, 128, 128, 255})
					}
				}
			}
			for y := 0; y < im.Bounds().Dy(); y++ {
				for x := 0; x < im.Bounds().Dx(); x++ {
					require.Equal(t, color.RGBAModel.Convert(expected.At(x, y)), color.RGBAModel.Convert(im.At(x, y)), "pixel %d,%d", x, y)
				}
			}
			require.Len(t, d.Meta.Tags, 1)
			require.Equal(t, 0, d.Meta.Tags[0].From)
			require.Equal(t, 1, d.Meta.Tags[0].To)
			require.Equal(t, "reverse", d.Meta.Tags[0].Direction)
			sourceUnchanged(t, p, before, stat)
			require.Empty(t, historyOperations(f, p))
			f.lua(p, `local s=app.activeSprite;local a=s.layers[1].layers[1];assert(a:cel(1).image==a:cel(3).image);assert(a:cel(1).position==Point(3,5));assert(s.data=='keep' and a.data=='paint metadata' and a:cel(1).data=='cel metadata' and s.properties('artist').text=='preserve');assert(not s.layers[1].layers[2].isVisible)`)
		})
	}
}

func TestExportOptionsHiddenEmptyInvalidAndStale(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeRGB)
	s := readStructure(t, f, map[string]any{"sprite_path": p})
	dest := filepath.Join(t.TempDir(), "out.png")
	base := func() map[string]any {
		return map[string]any{"sprite_path": p, "output_path": dest, "format": "png", "frame_number": 0, "layer_id": "1/1", "expected_revision": s.Revision}
	}
	for _, tc := range []struct {
		change map[string]any
		reason string
	}{
		{map[string]any{"layer_id": "1/2"}, "selection is empty"},
		{map[string]any{"layer_id": "3"}, "selection is empty"},
		{map[string]any{"layer_id": "99"}, "layer not found"},
		{map[string]any{"tag": "missing"}, "tag not found"},
		{map[string]any{"frame_start": 4}, "out of bounds"},
		{map[string]any{"frame_start": 0}, "frame endpoints"},
		{map[string]any{"frame_start": 3, "frame_end": 2}, "ordered"},
		{map[string]any{"tag": "walk/\"\n", "frame_end": 2}, "mutually exclusive"},
		{map[string]any{"frame_number": 1, "frame_end": 2}, "mutually exclusive"},
		{map[string]any{"expected_revision": ""}, "requires"},
		{map[string]any{"layer_id": "0"}, "requires"},
		{map[string]any{"tag": ""}, "must not be empty"},
		{map[string]any{"layer_id": "", "include_hidden": true}, "requires layer_id"},
		{map[string]any{"format": "gif", "output_path": filepath.Join(t.TempDir(), "x.gif")}, "GIF does not support"},
	} {
		args := base()
		for k, v := range tc.change {
			args[k] = v
		}
		rejectExport(t, f, "export_sprite", p, args, tc.reason)
	}
	args := base()
	args["layer_id"] = "1/2"
	args["include_hidden"] = true
	args["frame_number"] = 1
	args["trim"] = true
	f.call("export_sprite", args)
	im := decodeExportPNG(t, dest)
	require.Equal(t, image.Rect(0, 0, 1, 1), im.Bounds())
	require.Equal(t, color.NRGBA{255, 255, 255, 255}, color.NRGBAModel.Convert(im.At(0, 0)))
	// A reordered hierarchy has different ID meaning. Old revision must reject.
	f.lua(p, `local s=app.activeSprite;s.layers[1].layers[1].stackIndex=2;s:saveAs(s.filename)`)
	rejectExport(t, f, "export_sprite", p, base(), "stale sprite revision")
	s = readStructure(t, f, map[string]any{"sprite_path": p})
	f.lua(p, `local s=app.activeSprite;local name=s.tags[1].name;local t=s:newTag(1,1);t.name=name;s:saveAs(s.filename)`)
	args = base()
	delete(args, "expected_revision")
	delete(args, "layer_id")
	args["tag"] = "walk/\"\n"
	rejectExport(t, f, "export_sprite", p, args, "Ambiguous export tag")
}

func TestExportOptionsSheetPaddingLayoutsAndLegacySidecar(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeRGB)
	for _, layout := range []string{"horizontal", "vertical", "rows", "columns", "packed"} {
		t.Run(layout, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sheet.png")
			out := f.call("export_spritesheet", map[string]any{"sprite_path": p, "output_path": path, "layout": layout, "padding": 4, "include_json": false, "frame_start": 2, "frame_end": 2, "trim": true, "border_padding": 0, "shape_padding": 0, "inner_padding": 1})
			require.NotContains(t, out, "metadata_path")
			require.Equal(t, float64(1), out["frame_count"])
			d := readSheet(t, spritesheetDataPath(path))
			im := decodeExportPNG(t, path)
			require.Len(t, d.Frames, 1)
			for _, fr := range d.Frames {
				require.Equal(t, sheetRect{0, 0, 4, 5}, fr.Frame)
				require.Equal(t, sheetRect{3, 5, 2, 3}, fr.Offset)
				for y := 0; y < 5; y++ {
					for x := 0; x < 4; x++ {
						want := color.NRGBA{}
						if x >= 1 && x <= 2 && y >= 1 && y <= 3 {
							want = color.NRGBA{128, 128, 128, 255}
						}
						require.Equal(t, want, color.NRGBAModel.Convert(im.At(x, y)))
					}
				}
			}
		})
	}
	for _, ext := range []string{"jpg", "gif", "bmp"} {
		path := filepath.Join(t.TempDir(), "sheet."+ext)
		f.call("export_spritesheet", map[string]any{"sprite_path": p, "output_path": path, "layout": "vertical", "padding": 0, "include_json": true})
		require.Len(t, readSheet(t, spritesheetDataPath(path)).Frames, 3)
	}
}

func TestExportOptionsOutputProtectionAndHistory(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeRGB)
	dir := t.TempDir()
	texture := filepath.Join(dir, "sheet.png")
	metadata := spritesheetDataPath(texture)
	args := map[string]any{"sprite_path": p, "output_path": texture, "layout": "horizontal", "padding": 0, "include_json": true}
	require.NoError(t, os.WriteFile(texture, []byte("old texture"), 0640))
	require.NoError(t, os.WriteFile(metadata, []byte("old metadata"), 0600))
	args["overwrite"] = false
	rejectExport(t, f, "export_spritesheet", p, args, "overwrite=false")
	delete(args, "overwrite")
	require.NoError(t, os.Chmod(metadata, 0400))
	rejectExport(t, f, "export_spritesheet", p, args, "read-only")
	require.NoError(t, os.Chmod(metadata, 0600))
	require.NoError(t, os.Remove(metadata))
	require.NoError(t, os.Mkdir(metadata, 0700))
	rejectExport(t, f, "export_spritesheet", p, args, "")
	require.NoError(t, os.Remove(metadata))
	require.NoError(t, os.Symlink(p, metadata))
	rejectExport(t, f, "export_spritesheet", p, args, "alias")
	require.NoError(t, os.Remove(metadata))
	require.NoError(t, os.Link(texture, metadata))
	rejectExport(t, f, "export_spritesheet", p, args, "alias")
	require.NoError(t, os.Remove(metadata))
	bytes, err := os.ReadFile(texture)
	require.NoError(t, err)
	require.Equal(t, "old texture", string(bytes))
	stage, err := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
	require.NoError(t, err)
	require.Empty(t, stage)
	// Existing extension aliases cannot select a different encoder in staging.
	wrong := filepath.Join(dir, "wrong.aseprite")
	require.NoError(t, os.WriteFile(wrong, []byte("keep"), 0600))
	require.NoError(t, os.Remove(texture))
	require.NoError(t, os.Symlink(wrong, texture))
	rejectExport(t, f, "export_spritesheet", p, args, "canonical output extensions")
	require.NoError(t, os.Remove(texture))
	// Export remains excluded from source edit history; later undo restores a
	// real edit, and manual snapshot restore still preserves the exact source.
	original, err := os.ReadFile(p)
	require.NoError(t, err)
	snap := f.call("create_snapshot", map[string]any{"sprite_path": p})
	f.call("add_layer", map[string]any{"sprite_path": p, "layer_name": "edit"})
	ops := historyOperations(f, p)
	require.Len(t, ops, 1)
	f.call("export_spritesheet", args)
	require.Equal(t, ops, historyOperations(f, p))
	f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": ops[0].(map[string]any)["operation_id"]})
	restored, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, original, restored)
	f.call("add_layer", map[string]any{"sprite_path": p, "layer_name": "snapshot-edit"})
	f.call("restore_snapshot", map[string]any{"sprite_path": p, "snapshot_id": snap["snapshot"].(map[string]any)["snapshot_id"]})
	restored, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, original, restored)
	// Cancellation while another caller owns a sidecar lock cannot publish texture.
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- aseprite.WithFileLocks(context.Background(), []string{metadata}, func(context.Context) error { close(ready); <-release; return nil })
	}()
	<-ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = exportSpritesheet(ctx, f.client, f.gen, ExportSpritesheetInput{SpritePath: p, OutputPath: texture})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	close(release)
	require.NoError(t, <-done)
}

func TestExportOptionsFramePalettes(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeIndexed)
	// Add a native per-frame palette chunk; the Lua setter only changes frame 1.
	// Index-zero pixels stay natively linked while their rendered colors differ.
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	start := 128 + int(binary.LittleEndian.Uint32(b[128:]))
	chunk := make([]byte, 32)
	binary.LittleEndian.PutUint32(chunk, 32)
	binary.LittleEndian.PutUint16(chunk[4:], 0x2019)
	binary.LittleEndian.PutUint32(chunk[6:], 4) // palette size; first=last=0
	copy(chunk[28:], []byte{255, 255, 255, 255})
	binary.LittleEndian.PutUint32(b[start:], binary.LittleEndian.Uint32(b[start:])+32)
	binary.LittleEndian.PutUint16(b[start+6:], binary.LittleEndian.Uint16(b[start+6:])+1)
	b = append(b[:start+16], append(chunk, b[start+16:]...)...)
	binary.LittleEndian.PutUint32(b, uint32(len(b)))
	require.NoError(t, os.WriteFile(p, b, 0600))
	f.lua(p, `local s=app.activeSprite;assert(#s.palettes==2);assert(s.palettes[2]:getColor(0).red==255);local a=s.layers[1].layers[1];assert(a:cel(1).image==a:cel(2).image)`)
	s := readStructure(t, f, map[string]any{"sprite_path": p})
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "palette.png"), "format": "png", "frame_number": 0, "frame_start": 1, "frame_end": 3, "layer_id": "1/1", "expected_revision": s.Revision, "trim": true})
	for i, file := range out["files"].([]any) {
		im := decodeExportPNG(t, file.(map[string]any)["path"].(string))
		v := uint8(255)
		if i == 0 {
			v = 128
		}
		require.Equal(t, color.NRGBA{v, v, v, 255}, color.NRGBAModel.Convert(im.At(0, 0)))
	}
	sheet := filepath.Join(t.TempDir(), "sheet.png")
	f.call("export_spritesheet", map[string]any{"sprite_path": p, "output_path": sheet, "layout": "horizontal", "padding": 0, "include_json": true, "frame_start": 1, "frame_end": 3, "trim": true})
	im := decodeExportPNG(t, sheet)
	d := readSheet(t, spritesheetDataPath(sheet))
	for _, fr := range d.Frames {
		v := uint8(255)
		if fr.Number == 1 {
			v = 128
		}
		require.Equal(t, color.NRGBA{v, v, v, 255}, color.NRGBAModel.Convert(im.At(fr.Frame.X, fr.Frame.Y)))
	}
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestExportOptionsBlankFrameAndSchema(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;s:deleteCel(s.layers[1].layers[1]:cel(2));s:saveAs(s.filename)`)
	out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "blank.png"), "format": "png", "frame_number": 0, "frame_start": 1, "trim": true, "overwrite": nil, "frame_end": nil})
	files := out["files"].([]any)
	require.Len(t, files, 3)
	im := decodeExportPNG(t, files[1].(map[string]any)["path"].(string))
	require.Equal(t, image.Rect(0, 0, 1, 1), im.Bounds())
	_, _, _, a := im.At(0, 0).RGBA()
	require.Zero(t, a)
	path := filepath.Join(t.TempDir(), "sheet.png")
	f.call("export_spritesheet", map[string]any{"sprite_path": p, "output_path": path, "layout": "horizontal", "padding": 0, "include_json": true, "trim": true, "frame_start": 1, "border_padding": nil})
	d := readSheet(t, spritesheetDataPath(path))
	require.Len(t, d.Frames, 3)
	for _, fr := range d.Frames {
		if fr.Number == 2 {
			im = decodeExportPNG(t, path)
			_, _, _, a = im.At(fr.Frame.X, fr.Frame.Y).RGBA()
			require.Zero(t, a)
			require.Equal(t, 200, fr.Duration)
		}
	}
	list, err := f.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	for _, tool := range list.Tools {
		if tool.Name != "export_sprite" && tool.Name != "export_spritesheet" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		require.NoError(t, err)
		var schema struct {
			Required   []string
			Properties map[string]any
		}
		require.NoError(t, json.Unmarshal(raw, &schema))
		want := []string{"sprite_path", "output_path", "format", "frame_number"}
		if tool.Name == "export_spritesheet" {
			want = []string{"sprite_path", "output_path", "layout", "padding", "include_json"}
		}
		require.ElementsMatch(t, want, schema.Required)
		for _, name := range []string{"tag", "layer_id", "expected_revision", "frame_start", "frame_end", "trim", "overwrite", "include_hidden"} {
			require.Contains(t, schema.Properties, name)
		}
	}
	for _, v := range []any{1.5, "1"} {
		r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_sprite", Arguments: map[string]any{"sprite_path": p, "output_path": path, "format": "png", "frame_number": 0, "frame_start": v}})
		require.Error(t, err)
		require.Nil(t, r)
	}
}

func TestExportOptionsSheetStageCancellationAndSourceChange(t *testing.T) {
	for _, change := range []bool{false, true} {
		t.Run(fmt.Sprint(change), func(t *testing.T) {
			f := newExportOptionsFixture(t)
			p := optionsSprite(t, f, aseprite.ColorModeRGB)
			dir := t.TempDir()
			paths := []string{filepath.Join(dir, "sheet.png"), filepath.Join(dir, "sheet.json")}
			for _, p := range paths {
				require.NoError(t, os.WriteFile(p, []byte("old"), 0640))
			}
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			err = aseprite.WithFileLocks(ctx, append([]string{p}, paths...), func(ctx context.Context) error {
				return aseprite.WithSpriteAccess(ctx, p, false, func(ctx context.Context) error {
					rev, e := aseprite.SpriteRevision(ctx, p)
					if e != nil {
						return e
					}
					return aseprite.WithOutputFiles(ctx, paths, func(staged []string) error {
						_, e := f.client.ExecuteLua(ctx, f.gen.ExportSelectedSheet(staged[0], staged[1], "horizontal", 0, 0, 0, false, false, aseprite.ExportSelection{}), p)
						if e != nil {
							return e
						}
						for _, path := range staged {
							st, e := os.Stat(path)
							if e != nil {
								return e
							}
							require.Positive(t, st.Size())
						}
						if change {
							if e = os.WriteFile(p, append(before, 0), 0600); e != nil {
								return e
							}
						} else {
							cancel()
						}
						return verifyExportRevision(ctx, p, rev)
					})
				})
			})
			if change {
				require.ErrorContains(t, err, "source changed")
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			for _, p := range paths {
				b, e := os.ReadFile(p)
				require.NoError(t, e)
				require.Equal(t, "old", string(b))
			}
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			want := before
			if change {
				want = append(want, 0)
			}
			require.Equal(t, want, after)
			staged, err := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
			require.NoError(t, err)
			require.Empty(t, staged)
		})
	}
}

func TestExportOptionsSheetInvalidAndMultiLayouts(t *testing.T) {
	f := newExportOptionsFixture(t)
	p := optionsSprite(t, f, aseprite.ColorModeRGB)
	s := readStructure(t, f, map[string]any{"sprite_path": p})
	base := func() map[string]any {
		return map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "sheet.png"), "layout": "horizontal", "padding": 0, "include_json": true}
	}
	for _, tc := range []struct {
		change map[string]any
		reason string
	}{
		{map[string]any{"border_padding": -1}, "individual padding"},
		{map[string]any{"shape_padding": 101}, "individual padding"},
		{map[string]any{"inner_padding": -1}, "individual padding"},
		{map[string]any{"tag": "not there"}, "tag not found"},
		{map[string]any{"frame_start": 0}, "frame endpoints"},
		{map[string]any{"frame_end": 4}, "out of bounds"},
		{map[string]any{"layer_id": "9", "expected_revision": s.Revision}, "layer not found"},
		{map[string]any{"layer_id": "1/2", "expected_revision": s.Revision}, "selection is empty"},
		{map[string]any{"expected_revision": strings.Repeat("0", 64)}, "stale sprite revision"},
		{map[string]any{"output_path": filepath.Join(t.TempDir(), "native.aseprite")}, "texture extension"},
	} {
		args := base()
		for k, v := range tc.change {
			args[k] = v
		}
		rejectExport(t, f, "export_spritesheet", p, args, tc.reason)
		_, err := os.Stat(args["output_path"].(string))
		require.True(t, os.IsNotExist(err))
	}
	for _, layout := range []string{"horizontal", "vertical", "rows", "columns", "packed"} {
		args := base()
		args["layout"] = layout
		args["frame_start"] = 2
		args["frame_end"] = 3
		args["trim"] = true
		f.call("export_spritesheet", args)
		d := readSheet(t, spritesheetDataPath(args["output_path"].(string)))
		require.Len(t, d.Frames, 2)
		for _, fr := range d.Frames {
			require.Contains(t, []int{2, 3}, fr.Number)
			require.Equal(t, 100*fr.Number, fr.Duration)
		}
	}
	// A wide but tiny-memory native document exercises real planner rejection.
	wide := filepath.Join(t.TempDir(), "wide.aseprite")
	f.lua("", fmt.Sprintf(`local s=Sprite(32768,1);s:saveAs("%s")`, aseprite.EscapeString(wide)))
	args := base()
	args["sprite_path"] = wide
	rejectExport(t, f, "export_spritesheet", wide, args, "dimension budget")
	missing := base()
	missing["sprite_path"] = filepath.Join(t.TempDir(), "missing.aseprite")
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_spritesheet", Arguments: missing})
	require.NoError(t, err)
	require.True(t, r.IsError)
}

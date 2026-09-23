//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func newExportBehavior(t *testing.T) *behaviorFixture {
	_, session, client, _ := createExportTestSession(t)
	t.Cleanup(func() { session.Close() })
	return &behaviorFixture{t, session, client, aseprite.NewLuaGenerator()}
}
func exportFixture(t *testing.T, f *behaviorFixture) string {
	t.Helper()
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite
local im=Image(4,4,ColorMode.RGB);im:clear(app.pixelColor.rgba(255,0,0,255));s:newCel(s.layers[1],1,im,Point(3,5))
s:newEmptyFrame();im:clear(app.pixelColor.rgba(0,0,255,255));s:newCel(s.layers[1],2,im,Point(3,5))
local g=s:newGroup();g.name="group";s.layers[1].parent=g
local hidden=s:newLayer();hidden.isVisible=false;im:clear(app.pixelColor.rgba(0,255,0,255));s:newCel(hidden,1,im,Point(3,5))
s.data="preserve source";s:saveAs(s.filename)`)
	return p
}
func checkExportPNG(t *testing.T, p string, red bool) {
	t.Helper()
	f, e := os.Open(p)
	require.NoError(t, e)
	defer f.Close()
	im, e := png.Decode(f)
	require.NoError(t, e)
	require.Equal(t, 16, im.Bounds().Dx())
	require.Equal(t, 16, im.Bounds().Dy())
	_, _, _, a := im.At(0, 0).RGBA()
	require.Zero(t, a)
	r, g, b, a := im.At(4, 6).RGBA()
	require.Equal(t, uint32(65535), a)
	require.Zero(t, g)
	if red {
		require.Equal(t, uint32(65535), r)
		require.Zero(t, b)
	} else {
		require.Zero(t, r)
		require.Equal(t, uint32(65535), b)
	}
}
func TestExportSequencePNGContract(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	before, e := os.ReadFile(p)
	require.NoError(t, e)
	dir := t.TempDir()
	base := filepath.Join(dir, "walk007.png")
	first := filepath.Join(dir, "walk007_0001.png")
	second := filepath.Join(dir, "walk007_0002.png")
	require.NoError(t, os.WriteFile(base, []byte("unrelated base"), 0600))
	require.NoError(t, os.WriteFile(first, []byte("old first"), 0640))
	orphan := filepath.Join(dir, "walk007_0009.png")
	require.NoError(t, os.WriteFile(orphan, []byte("unrelated old frame"), 0600))
	out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": base, "format": "png", "frame_number": 0})
	require.Equal(t, first, out["exported_path"])
	files, ok := out["files"].([]any)
	require.True(t, ok)
	require.Len(t, files, 2)
	for i, path := range []string{first, second} {
		entry := files[i].(map[string]any)
		require.Equal(t, path, entry["path"])
		require.Equal(t, float64(i+1), entry["frame_number"])
		st, e := os.Stat(path)
		require.NoError(t, e)
		require.Equal(t, float64(st.Size()), entry["file_size"])
		if i == 0 {
			require.Equal(t, float64(st.Size()), out["file_size"])
		}
		checkExportPNG(t, path, i == 0)
	}
	after, e := os.ReadFile(p)
	require.NoError(t, e)
	require.Equal(t, before, after)
	data, e := os.ReadFile(base)
	require.NoError(t, e)
	require.Equal(t, "unrelated base", string(data))
	data, e = os.ReadFile(orphan)
	require.NoError(t, e)
	require.Equal(t, "unrelated old frame", string(data))
	dirs, e := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
	require.NoError(t, e)
	require.Empty(t, dirs)
}
func TestExportSequenceSingleFrameCompatibility(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	dest := filepath.Join(t.TempDir(), "chosen.png")
	out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": dest, "format": "png", "frame_number": 2})
	require.Equal(t, dest, out["exported_path"])
	require.NotContains(t, out, "files")
	checkExportPNG(t, dest, false)
}
func TestExportSequenceInvalidDestinationPreservesFiles(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	before, e := os.ReadFile(p)
	require.NoError(t, e)
	dir := t.TempDir()
	base := filepath.Join(dir, "out.png")
	first := filepath.Join(dir, "out_0001.png")
	second := filepath.Join(dir, "out_0002.png")
	require.NoError(t, os.WriteFile(first, []byte("old first"), 0600))
	require.NoError(t, os.Mkdir(second, 0700))
	r, e := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_sprite", Arguments: map[string]any{"sprite_path": p, "output_path": base, "format": "png", "frame_number": 0}})
	require.NoError(t, e)
	require.True(t, r.IsError)
	data, e := os.ReadFile(first)
	require.NoError(t, e)
	require.Equal(t, "old first", string(data))
	after, e := os.ReadFile(p)
	require.NoError(t, e)
	require.Equal(t, before, after)
}

func TestExportSequenceOtherFormatsAndSingleCanvas(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	for _, format := range []string{"jpg", "bmp", "gif"} {
		t.Run(format, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "sprite."+format)
			out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": dest, "format": format, "frame_number": 0})
			if format == "gif" {
				require.Equal(t, dest, out["exported_path"])
				require.NotContains(t, out, "files")
				f.lua("", fmt.Sprintf(`local s=app.open("%s");assert(#s.frames==2);s:close()`, aseprite.EscapeString(dest)))
			} else {
				files := out["files"].([]any)
				require.Len(t, files, 2)
				for _, entry := range files {
					path := entry.(map[string]any)["path"].(string)
					data, err := os.ReadFile(path)
					require.NoError(t, err)
					require.NotEmpty(t, data)
					if format == "bmp" {
						require.Equal(t, "BM", string(data[:2]))
					} else {
						require.Equal(t, []byte{255, 216}, data[:2])
					}
				}
			}
			// A selected frame must remain exactly one output for each format.
			out = f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": dest, "format": format, "frame_number": 2})
			require.NotContains(t, out, "files")
			require.Equal(t, dest, out["exported_path"])
		})
	}
	single := f.sprite(aseprite.ColorModeRGB)
	dest := filepath.Join(t.TempDir(), "still.png")
	out := f.call("export_sprite", map[string]any{"sprite_path": single, "output_path": dest, "format": "png", "frame_number": 0})
	require.Equal(t, dest, out["exported_path"])
	require.NotContains(t, out, "files")
}
func TestExportSequenceValidationAndSchema(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	for _, tc := range []struct {
		format, path string
		frame        int
	}{
		{"png", filepath.Join(t.TempDir(), "wrong.gif"), 0},
		{"png", filepath.Join(t.TempDir(), "out.png"), 3},
		{"png", filepath.Join(t.TempDir(), "out.png"), -1},
	} {
		result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_sprite", Arguments: map[string]any{"sprite_path": p, "output_path": tc.path, "format": tc.format, "frame_number": tc.frame}})
		require.NoError(t, err)
		require.True(t, result.IsError)
		after, err := os.ReadFile(p)
		require.NoError(t, err)
		require.Equal(t, before, after)
		_, err = os.Stat(tc.path)
		require.True(t, os.IsNotExist(err))
	}
	// An output symlink must never replace or modify the input sprite.
	dest := filepath.Join(t.TempDir(), "alias.png")
	require.NoError(t, os.Symlink(p, dest))
	result, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "export_sprite", Arguments: map[string]any{"sprite_path": p, "output_path": dest, "format": "png", "frame_number": 1}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	list, err := f.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	found := false
	for _, tool := range list.Tools {
		if tool.Name == "export_sprite" {
			found = true
			data, err := json.Marshal(tool.OutputSchema)
			require.NoError(t, err)
			var schema struct {
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			}
			require.NoError(t, json.Unmarshal(data, &schema))
			require.Contains(t, schema.Properties, "files")
			require.NotContains(t, schema.Required, "files")
			require.Contains(t, schema.Required, "exported_path")
			require.Contains(t, schema.Required, "file_size")
		}
	}
	require.True(t, found)
}
func TestExportSequenceIndexedPixels(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	f.lua(p, `local s=app.activeSprite;local pal=s.palettes[1];pal:resize(4);pal:setColor(0,Color{r=0,g=0,b=0,a=0});pal:setColor(1,Color{r=255,g=0,b=0,a=255});pal:setColor(2,Color{r=0,g=0,b=255,a=255});pal:setColor(3,Color{r=0,g=255,b=0,a=255});app.command.ChangePixelFormat{ui=false,format="indexed"};s:saveAs(s.filename)`)
	out := f.call("export_sprite", map[string]any{"sprite_path": p, "output_path": filepath.Join(t.TempDir(), "indexed.png"), "format": "png", "frame_number": 0})
	for i, file := range out["files"].([]any) {
		checkExportPNG(t, file.(map[string]any)["path"].(string), i == 0)
	}
}

func TestExportSequenceStalePlanWritesNothing(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	dest := filepath.Join(t.TempDir(), "stale.png")
	_, err = f.client.ExecuteLua(context.Background(), f.gen.ExportSpriteFiles([]string{dest}, []int{1}, 3), p)
	require.ErrorContains(t, err, "file_changed")
	_, err = os.Stat(dest)
	require.True(t, os.IsNotExist(err))
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestExportSequenceRejectsReportedSaveFailure(t *testing.T) {
	f := newExportBehavior(t)
	p := exportFixture(t, f)
	dest := filepath.Join(t.TempDir(), "partial.png")
	// Exercise the actual generated Lua with a save API that returns false after
	// leaving non-empty bytes. File existence alone must not imply success.
	script := `local Image=function(...) return {
 drawSprite=function() end,
 saveAs=function(self,path) local file=assert(io.open(path,"wb"));file:write("partial");file:close();return false end
 } end
` + f.gen.ExportSpriteFiles([]string{dest}, []int{1}, 2)
	_, err := f.client.ExecuteLua(context.Background(), script, p)
	require.ErrorContains(t, err, "Failed to save export frame")
}

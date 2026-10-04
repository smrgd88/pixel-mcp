//go:build integration

package tools

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func newStructureFixture(t *testing.T) (*behaviorFixture, string) {
	cfg := testutil.LoadTestConfig(t)
	cfg.TempDir = t.TempDir()
	cfg.EnableHistory = true
	cfg.SnapshotDir = filepath.Join(t.TempDir(), "history")
	c := aseprite.NewClient(cfg.AsepritePath, cfg.TempDir, cfg.Timeout)
	g := aseprite.NewLuaGenerator()
	logger := mtlog.New(mtlog.WithMinimumLevel(core.ErrorLevel))
	s := mcp.NewServer(&mcp.Implementation{Name: "structure-test", Version: "1"}, nil)
	RegisterInspectionTools(s, c, g, cfg, logger)
	RegisterCanvasTools(s, c, g, cfg, logger)
	RegisterHistoryTools(s, cfg, logger)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.Connect(context.Background(), st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(context.Background(), ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close(); ss.Close() })
	return &behaviorFixture{t, cs, c, g}, cfg.SnapshotDir
}
func readStructure(t *testing.T, f *behaviorFixture, args map[string]any) GetSpriteStructureOutput {
	t.Helper()
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_structure", Arguments: args})
	require.NoError(t, err)
	require.False(t, r.IsError, "%+v", r.Content)
	var out GetSpriteStructureOutput
	require.NoError(t, json.Unmarshal([]byte(r.Content[0].(*mcp.TextContent).Text), &out))
	return out
}
func TestSpriteStructureSavedModes(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f, store := newStructureFixture(t)
			p := f.sprite(mode)
			f.lua(p, `local s=app.activeSprite
local original=s.layers[1]
local group=s:newGroup();group.name='same/"\n한글'
s:deleteLayer(original)
local nested=s:newGroup();nested.name=group.name;nested.parent=group
local a=s:newLayer();a.name="paint";a.parent=nested;a.opacity=123
local b=s:newLayer();b.name="paint";b.parent=nested;b.isVisible=false;b.isEditable=false
local empty=s:newLayer();empty.name="empty";empty.parent=group
local im=Image(2,3,s.colorMode)
if s.colorMode==ColorMode.INDEXED then s.transparentColor=3;im:clear(1)
elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255))
else im:clear(app.pixelColor.rgba(128,128,128,255)) end
s:newCel(a,1,im,Point(-2,5));s:newEmptyFrame();s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
a:cel(1).opacity=91
s:newCel(b,1,Image(im),Point(7,-3));b:cel(1).opacity=173
group.isVisible=false;group.isEditable=false
s.data="source metadata";a.data="layer metadata";a:cel(1).data="cel metadata"
s.properties("artist").text="keep";s.frames[2].duration=0.25
s:saveAs(s.filename)`)
			// Linked setters propagate position/opacity through shared CelData in the
			// Lua API. The native format permits per-cel headers: patch only the linked
			// header in this generated test fixture, then let the real decoder reopen it.
			patchStructureLinkedHeader(t, p, false)
			// Reopen independently and verify the fixture's native relation, equal pixels,
			// distinct cel properties, and user metadata before asking the MCP tool.
			inspect := `local s=app.activeSprite;local a=s.layers[1].layers[1].layers[1];local b=s.layers[1].layers[1].layers[2]
assert(a:cel(1).image==a:cel(2).image)
assert(a:cel(1).image~=b:cel(1).image and a:cel(1).image.bytes==b:cel(1).image.bytes)
assert(a:cel(1).position==Point(-2,5) and a:cel(2).position==Point(-2,5),json.encode({x1=a:cel(1).position.x,y1=a:cel(1).position.y,x2=a:cel(2).position.x,y2=a:cel(2).position.y}))
assert(a:cel(1).opacity==91 and a:cel(2).opacity==91 and a:cel(2).zIndex==2)
assert(s.data=="source metadata" and a.data=="layer metadata" and a:cel(1).data=="cel metadata")
assert(s.properties("artist").text=="keep" and math.abs(s.frames[2].duration-0.25)<0.001)`
			f.lua(p, inspect)
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			stat, err := os.Stat(p)
			require.NoError(t, err)
			args := map[string]any{"sprite_path": p}
			out := readStructure(t, f, args)
			require.Equal(t, 5, out.LayerCount)
			require.Equal(t, 3, out.FrameCount)
			require.Equal(t, string(mode), out.ColorMode)
			require.Len(t, out.Layers, 5)
			require.Equal(t, []string{"1", "1/1", "1/1/1", "1/1/2", "1/2"}, []string{out.Layers[0].LayerID, out.Layers[1].LayerID, out.Layers[2].LayerID, out.Layers[3].LayerID, out.Layers[4].LayerID})
			require.Equal(t, "same/\"\n한글", out.Layers[0].Name)
			require.Equal(t, "group", out.Layers[0].Kind)
			require.Nil(t, out.Layers[0].Opacity)
			require.Empty(t, out.Layers[0].Cels)
			require.NotNil(t, out.Layers[0].Cels)
			a, b := out.Layers[2], out.Layers[3]
			require.Equal(t, "1/1", a.ParentID)
			require.Len(t, a.NamePath, 3)
			require.Equal(t, "raster", a.Kind)
			require.Equal(t, 123, *a.Opacity)
			require.True(t, a.Visible)
			require.True(t, a.Editable)
			require.False(t, a.EffectiveVisible)
			require.False(t, a.EffectiveEditable)
			require.False(t, b.Visible)
			require.False(t, b.Editable)
			require.Len(t, a.Cels, 3)
			require.Equal(t, -2, *a.Cels[0].X)
			require.Equal(t, 5, *a.Cels[0].Y)
			require.Equal(t, 2, *a.Cels[0].Width)
			require.Equal(t, 3, *a.Cels[0].Height)
			require.Equal(t, 91, *a.Cels[0].Opacity)
			require.Equal(t, 91, *a.Cels[1].Opacity)
			require.Equal(t, -2, *a.Cels[1].X)
			require.Equal(t, 5, *a.Cels[1].Y)
			require.Equal(t, 2, *a.Cels[1].ZIndex)
			require.Equal(t, "1/1/1@1", a.Cels[0].ImageRef)
			require.Equal(t, a.Cels[0].ImageRef, a.Cels[1].ImageRef)
			require.Equal(t, 2, a.Cels[1].LinkedCelCount)
			require.Equal(t, 7, *b.Cels[0].X)
			require.Equal(t, -3, *b.Cels[0].Y)
			require.Equal(t, 173, *b.Cels[0].Opacity)
			require.NotEqual(t, a.Cels[0].ImageRef, b.Cels[0].ImageRef)
			require.Equal(t, 1, b.Cels[0].LinkedCelCount)
			require.False(t, a.Cels[2].Exists)
			require.Nil(t, a.Cels[2].X)
			require.Empty(t, a.Cels[2].ImageRef)
			for _, c := range out.Layers[4].Cels {
				require.False(t, c.Exists)
			}
			require.Equal(t, out, readStructure(t, f, args), "new batch process must produce the same structural IDs")
			filtered := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": a.LayerID, "frame_start": 2, "frame_end": 2})
			require.Len(t, filtered.Layers, 1)
			require.Len(t, filtered.Layers[0].Cels, 1)
			require.Equal(t, a.Cels[1], filtered.Layers[0].Cels[0])
			require.Equal(t, 3, filtered.NextFrameStart)
			page := readStructure(t, f, map[string]any{"sprite_path": p, "page_size": 2})
			require.Equal(t, 2, *page.NextLayerOffset)
			page = readStructure(t, f, map[string]any{"sprite_path": p, "page_size": 2, "layer_offset": 2})
			require.Equal(t, a, page.Layers[0])
			require.Equal(t, 4, *page.NextLayerOffset)
			end := readStructure(t, f, map[string]any{"sprite_path": p, "layer_offset": 5})
			require.Empty(t, end.Layers)
			require.NotNil(t, end.Layers)
			require.Nil(t, end.NextLayerOffset)
			for _, extra := range []map[string]any{{"layer_id": "1/99"}, {"frame_start": 4}, {"frame_end": 4}, {"layer_offset": 6}, {"page_size": 101}, {"layer_id": "paint"}, {"frame_end": 101}} {
				extra["sprite_path"] = p
				r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_structure", Arguments: extra})
				require.NoError(t, err)
				require.True(t, r.IsError, "%v", extra)
			}
			f.lua(p, inspect)
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterStat, err := os.Stat(p)
			require.NoError(t, err)
			require.True(t, os.SameFile(stat, afterStat))
			require.Equal(t, stat.ModTime(), afterStat.ModTime())
			require.Equal(t, stat.Mode(), afterStat.Mode())
			_, err = os.Stat(store)
			require.True(t, os.IsNotExist(err), "read-only must not initialize history storage")
			require.Empty(t, historyOperations(f, p))
		})
	}
}

func TestSpriteStructureFrameWindowAndSchema(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;for i=2,102 do s:newEmptyFrame() end;for i=2,101 do s:newLayer() end;s:saveAs(s.filename)`)
	out := readStructure(t, f, map[string]any{"sprite_path": p})
	require.Equal(t, 101, out.LayerCount)
	require.Len(t, out.Layers, 50)
	require.Equal(t, 50, *out.NextLayerOffset)
	require.Equal(t, 100, out.FrameEnd)
	require.Equal(t, 101, out.NextFrameStart)
	require.Len(t, out.Layers[0].Cels, 100)
	maximum := readStructure(t, f, map[string]any{"sprite_path": p, "page_size": 100})
	require.Len(t, maximum.Layers, 100)
	slots := 0
	for _, l := range maximum.Layers {
		slots += len(l.Cels)
	}
	require.Equal(t, 10000, slots)
	require.Equal(t, 100, *maximum.NextLayerOffset)
	finalPage := readStructure(t, f, map[string]any{"sprite_path": p, "page_size": 100, "layer_offset": 100})
	require.Len(t, finalPage.Layers, 1)
	require.Nil(t, finalPage.NextLayerOffset)
	last := readStructure(t, f, map[string]any{"sprite_path": p, "frame_start": 101})
	require.Equal(t, 102, last.FrameEnd)
	require.Zero(t, last.NextFrameStart)
	require.Len(t, last.Layers[0].Cels, 2)
	list, err := f.session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	found := false
	for _, tool := range list.Tools {
		if tool.Name == "get_sprite_structure" {
			b, err := json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			var schema struct {
				Required []string `json:"required"`
			}
			require.NoError(t, json.Unmarshal(b, &schema))
			require.Equal(t, []string{"sprite_path"}, schema.Required)
			found = true
		}
	}
	require.True(t, found)
	for _, args := range []map[string]any{{}, {"sprite_path": p, "page_size": 1.5}, {"sprite_path": p, "frame_start": "2"}} {
		r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_sprite_structure", Arguments: args})
		require.Error(t, err, fmt.Sprint(args))
		require.Nil(t, r)
	}
}

func patchStructureLinkedHeader(t *testing.T, path string, legacy bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	patched := 0
	for frame, start := 1, 128; start < len(b); frame++ {
		require.LessOrEqual(t, start+16, len(b))
		end := start + int(binary.LittleEndian.Uint32(b[start:]))
		require.LessOrEqual(t, end, len(b))
		for chunk := start + 16; chunk < end; {
			require.LessOrEqual(t, chunk+6, end)
			size := int(binary.LittleEndian.Uint32(b[chunk:]))
			require.GreaterOrEqual(t, size, 6)
			require.LessOrEqual(t, chunk+size, end)
			if binary.LittleEndian.Uint16(b[chunk+4:]) == 0x2005 && frame == 2 {
				data := b[chunk+6 : chunk+size]
				require.GreaterOrEqual(t, len(data), 18)
				require.Equal(t, uint16(1), binary.LittleEndian.Uint16(data[7:])) // linked cel
				if legacy {
					binary.LittleEndian.PutUint16(data[2:], 7)
					binary.LittleEndian.PutUint16(data[4:], uint16(65533))
					data[6] = 173
				}
				binary.LittleEndian.PutUint16(data[9:], 2)
				patched++
			}
			chunk += size
		}
		start = end
	}
	require.Equal(t, 1, patched)
	require.NoError(t, os.WriteFile(path, b, 0600))
}

// A historical linked chunk with unequal position/opacity decodes as a copy.
// The tool reports native runtime sharing, not the on-disk link chunk alone.
func TestSpriteStructureLegacyLinkDecodesAsCopy(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(mode)
			f.lua(p, `local s=app.activeSprite;local a=s.layers[1]
local im=Image(2,3,s.colorMode);im:clear(1);s:newCel(a,1,im,Point(-2,5));s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels();s:saveAs(s.filename)`)
			patchStructureLinkedHeader(t, p, true)
			before, err := os.ReadFile(p)
			require.NoError(t, err)
			out := readStructure(t, f, map[string]any{"sprite_path": p})
			c := out.Layers[0].Cels
			require.True(t, c[0].Exists)
			require.True(t, c[1].Exists)
			require.NotEqual(t, c[0].ImageRef, c[1].ImageRef)
			require.Equal(t, 1, c[0].LinkedCelCount)
			require.Equal(t, 1, c[1].LinkedCelCount)
			require.Equal(t, -2, *c[0].X)
			require.Equal(t, 7, *c[1].X)
			require.Equal(t, -3, *c[1].Y)
			require.Equal(t, 173, *c[1].Opacity)
			f.lua(p, `local a=app.activeSprite.layers[1];assert(a:cel(1).image~=a:cel(2).image and a:cel(1).image.bytes==a:cel(2).image.bytes)`)
			after, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

// Keep zero-valued metadata distinct from absent fields and absent cels, and
// exercise structural IDs independently of names and lexical sorting.
func TestSpriteStructureEmptyNamesGroupsAndZeroValues(t *testing.T) {
	f, store := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite
local a=s.layers[1];a.name="";a.opacity=0
local im=Image(1,1);im:clear(app.pixelColor.rgba(1,2,3,255))
s:newCel(a,1,im,Point(0,0));a:cel(1).opacity=0
s:newEmptyFrame();s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[3]};app.command.LinkCels()
for i=2,9 do local layer=s:newLayer();layer.name="same" end
local group=s:newGroup();group.name="";group.isVisible=false
local child=s:newLayer();child.name="\t\\\"☃";child.parent=group
local empty=s:newGroup();empty.name="empty"
s:saveAs(s.filename)`)
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)

	out := readStructure(t, f, map[string]any{"sprite_path": p})
	require.Equal(t, 12, out.LayerCount)
	require.Len(t, out.Layers, 12)
	ids := make([]string, 0, len(out.Layers))
	for _, layer := range out.Layers {
		ids = append(ids, layer.LayerID)
	}
	require.Equal(t, []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "10/1", "11"}, ids)
	zero := 0
	layer := out.Layers[0]
	require.Empty(t, layer.Name)
	require.Equal(t, []string{""}, layer.NamePath)
	require.Equal(t, &zero, layer.Opacity)
	require.Len(t, layer.Cels, 3)
	cel := layer.Cels[0]
	require.True(t, cel.Exists, "opacity zero must not be treated as an absent cel")
	require.Equal(t, &zero, cel.X)
	require.Equal(t, &zero, cel.Y)
	require.Equal(t, &zero, cel.Opacity)
	require.Equal(t, &zero, cel.ZIndex)
	require.Equal(t, StructureCel{FrameNumber: 2, Exists: false}, layer.Cels[1])

	child := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "10/1"})
	require.Len(t, child.Layers, 1)
	require.Equal(t, "10", child.Layers[0].ParentID)
	require.Equal(t, []string{"", "\t\\\"☃"}, child.Layers[0].NamePath)
	require.True(t, child.Layers[0].Visible)
	require.False(t, child.Layers[0].EffectiveVisible)
	empty := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "11"})
	require.Len(t, empty.Layers, 1)
	require.Equal(t, "group", empty.Layers[0].Kind)
	require.Equal(t, []StructureCel{}, empty.Layers[0].Cels, "empty group must encode an array, not null")

	filtered := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "1", "frame_start": 3, "frame_end": 3})
	require.Len(t, filtered.Layers, 1)
	require.Len(t, filtered.Layers[0].Cels, 1)
	require.Equal(t, "1@1", filtered.Layers[0].Cels[0].ImageRef)
	require.Equal(t, 2, filtered.Layers[0].Cels[0].LinkedCelCount, "sharing includes the frame outside the filter")
	defaults := readStructure(t, f, map[string]any{"sprite_path": p, "page_size": 0, "frame_start": 0, "frame_end": 0})
	require.Equal(t, out, defaults)

	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	afterStat, err := os.Stat(p)
	require.NoError(t, err)
	require.True(t, os.SameFile(stat, afterStat))
	require.Equal(t, stat.ModTime(), afterStat.ModTime())
	require.Equal(t, stat.Mode(), afterStat.Mode())
	_, err = os.Stat(store)
	require.True(t, os.IsNotExist(err), "inspection must not initialize the history store")
}

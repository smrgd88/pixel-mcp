//go:build integration

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func celArgs(t *testing.T, f *behaviorFixture, p, layer string, frame int) map[string]any {
	t.Helper()
	s := readStructure(t, f, map[string]any{"sprite_path": p})
	require.Len(t, s.Revision, 64)
	return map[string]any{"sprite_path": p, "layer_id": layer, "frame_number": frame, "expected_revision": s.Revision}
}

func rejectCel(t *testing.T, f *behaviorFixture, p string, args map[string]any, reason ...string) {
	t.Helper()
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)
	ops := historyOperations(f, p)
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_cel_properties", Arguments: args})
	require.True(t, err != nil || r.IsError, "expected rejection: %+v", r)
	if len(reason) > 0 {
		require.NoError(t, err)
		require.NotEmpty(t, r.Content)
		require.Contains(t, r.Content[0].(*mcp.TextContent).Text, reason[0])
	}
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	now, err := os.Stat(p)
	require.NoError(t, err)
	require.True(t, os.SameFile(stat, now))
	require.Equal(t, stat.ModTime(), now.ModTime())
	require.Equal(t, stat.Mode(), now.Mode())
	require.Equal(t, ops, historyOperations(f, p))
}

func TestCelPropertiesSavedModesAndUndo(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(mode)
			f.lua(p, `local s=app.activeSprite;local root=s.layers[1];root.name="same"
local group=s:newGroup();group.name="same"
local a=s:newLayer();a.name="same";a.parent=group
local b=s:newLayer();b.name="same";b.parent=group
local im=Image(2,3,s.colorMode)
if s.colorMode==ColorMode.INDEXED then s.transparentColor=3;im:clear(1)
elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255))
else im:clear(app.pixelColor.rgba(128,128,128,255)) end
s:newCel(a,1,im,Point(1,2));s:newEmptyFrame();s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
a:cel(1).opacity=123;a:cel(2).zIndex=2
s:newCel(b,1,Image(im),Point(4,5));b:cel(1).opacity=201
s:newCel(root,1,Image(im),Point(6,7))
s.data="sprite";s.properties("artist").text="keep";a.data="layer";a:cel(1).data="cel"
s.frames[2].duration=0.25;group.isVisible=false
local tag=s:newTag(1,2);tag.name="keep"
s:saveAs(s.filename)`)
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			args := celArgs(t, f, p, "2/1", 1)
			args["x"] = 0
			args["y"] = -2
			args["opacity"] = 0
			rejectCel(t, f, p, args)
			args["allow_linked"] = true
			out := f.call("set_cel_properties", args)
			require.Equal(t, true, out["success"])
			require.Len(t, out["affected_cels"], 2)
			require.Len(t, out["warnings"], 1)
			require.NotEqual(t, args["expected_revision"], out["revision"])
			require.Equal(t, out["revision"], readStructure(t, f, map[string]any{"sprite_path": p}).Revision)
			f.lua(p, `local s=app.activeSprite;local a=s.layers[2].layers[1];local b=s.layers[2].layers[2]
assert(a:cel(1).image==a:cel(2).image and a:cel(1).image~=b:cel(1).image)
assert(a:cel(1).image.bytes==b:cel(1).image.bytes and a:cel(1).image.width==2 and a:cel(1).image.height==3)
assert(a:cel(1).position==Point(0,-2) and a:cel(2).position==Point(0,-2))
assert(a:cel(1).opacity==0 and a:cel(2).opacity==0 and a:cel(2).zIndex==2)
assert(b:cel(1).position==Point(4,5) and b:cel(1).opacity==201 and not a:cel(3))
assert(s.layers[1]:cel(1).position==Point(6,7) and #s.layers==2 and #s.layers[2].layers==2)
assert(s.data=="sprite" and s.properties("artist").text=="keep" and a.data=="layer" and a:cel(1).data=="cel")
assert(s.tags[1].name=="keep" and math.abs(s.frames[2].duration-0.25)<0.001 and not s.layers[2].isVisible)
if s.colorMode==ColorMode.INDEXED then assert(s.transparentColor==3 and a:cel(1).image:getPixel(0,0)==1) end`)
			rejectCel(t, f, p, args, "stale sprite revision") // duplicate request with old revision
			args["expected_revision"] = out["revision"]
			rejectCel(t, f, p, args) // same values, fresh revision
			ops := historyOperations(f, p)
			require.Len(t, ops, 1)
			op := ops[0].(map[string]any)
			require.Equal(t, "set_cel_properties", op["tool"])
			undo := f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]})
			require.NotEmpty(t, undo["backup_snapshot"])
			restored, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, original, restored)
			// Independent equal pixels are never included; hidden targets are editable.
			args = celArgs(t, f, p, "2/2", 1)
			args["x"] = -32768
			args["y"] = 32767
			out = f.call("set_cel_properties", args)
			require.Len(t, out["affected_cels"], 1)
			require.NotContains(t, out, "warnings")
			f.lua(p, `local s=app.activeSprite;local a=s.layers[2].layers[1];local b=s.layers[2].layers[2]
assert(b:cel(1).position==Point(-32768,32767) and b:cel(1).opacity==201)
assert(a:cel(1).position==Point(1,2) and a:cel(1).opacity==123 and a:cel(1).image==a:cel(2).image)`)
			args = celArgs(t, f, p, "1", 1)
			args["x"] = 0
			args["y"] = 0
			args["opacity"] = 255
			f.call("set_cel_properties", args)
			// Render in a separate batch after reopening. Root is the sole visible cel.
			f.lua(p, `local s=app.activeSprite;local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,1)
assert(app.pixelColor.rgbaA(im:getPixel(0,0))==255)
assert(app.pixelColor.rgbaA(im:getPixel(2,0))==0)`)
		})
	}
}

func TestCelPropertiesRejectionsAndStaleStructure(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local a=s.layers[1];a.name="same";local im=Image(1,1);im:clear(0xffffffff);s:newCel(a,1,im)
local g=s:newGroup();local b=s:newLayer();b.parent=g;b.name="same";s:newCel(b,1,Image(im));s:newEmptyFrame();s:saveAs(s.filename)`)
	for _, patch := range []map[string]any{
		{}, {"x": nil}, {"x": 1.5}, {"x": "1"}, {"x": 32768}, {"y": -32769}, {"x": 1e30}, {"opacity": -1}, {"opacity": 256},
		{"x": 1, "layer_id": "1/1"}, {"x": 1, "layer_id": "2"}, {"x": 1, "layer_id": "3"},
		{"x": 1, "frame_number": 0}, {"x": 1, "frame_number": 2}, {"x": 1, "frame_number": 3}, {"x": 1, "expected_revision": "bad"},
	} {
		args := celArgs(t, f, p, "1", 1)
		for k, v := range patch {
			args[k] = v
		}
		rejectCel(t, f, p, args)
	}
	for _, change := range []string{
		`s.layers[1].isEditable=false`,
		`s.layers[2].isEditable=false`,
	} {
		f.lua(p, `local s=app.activeSprite;`+change+`;s:saveAs(s.filename)`)
		layer := "1"
		if change == `s.layers[2].isEditable=false` {
			layer = "2/1"
		}
		args := celArgs(t, f, p, layer, 1)
		args["x"] = 1
		rejectCel(t, f, p, args)
	}
	// Same names and same ID now denote a different target; full bytes reject it.
	for _, change := range []string{
		`s.layers[1].stackIndex=2`,
		`local l=s:newLayer();l.name="same";l.stackIndex=1`,
		`s:deleteLayer(s.layers[#s.layers])`,
		`s:newEmptyFrame(1)`,
		`s:deleteFrame(1)`,
		`s.data="external metadata change"`,
	} {
		args := celArgs(t, f, p, "1", 1)
		args["x"] = 5
		f.lua(p, `local s=app.activeSprite;`+change+`;s:saveAs(s.filename)`)
		rejectCel(t, f, p, args, "stale sprite revision")
	}
}

func TestCelPropertiesConcurrentRevision(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local im=Image(1,1);im:clear(0xffffffff);s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
	args := celArgs(t, f, p, "1", 1)
	args["x"] = 2
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_cel_properties", Arguments: args})
			results <- err == nil && !r.IsError
		}()
	}
	wg.Wait()
	close(results)
	success := 0
	for ok := range results {
		if ok {
			success++
		}
	}
	require.Equal(t, 1, success)
	require.Len(t, historyOperations(f, p), 1)
	f.lua(p, fmt.Sprintf(`assert(app.activeSprite.layers[1]:cel(1).position.x==%d)`, 2))
}

// Exercise the same history/staging wrapper with real cel edits, then inject
// failure after Aseprite saved. No failed edit or backup may be published.
func TestCelPropertiesFailureAfterSave(t *testing.T) {
	for _, kind := range []string{"lua_error", "cancelled", "external_change", "backup_capacity"} {
		t.Run(kind, func(t *testing.T) {
			f, dir := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;local im=Image(1,1);im:clear(0xffffffff);s:newCel(s.layers[1],1,im);s:saveAs(s.filename)`)
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			store := aseprite.NewSnapshotStore(dir)
			if kind == "backup_capacity" {
				store.MaxCount = 1
				_, err = store.Create(context.Background(), p, "fill capacity")
				require.NoError(t, err)
			}
			x := 3
			in := SetCelPropertiesInput{SpritePath: p, LayerID: "1", FrameNumber: 1, X: &x, ExpectedRevision: readStructure(t, f, map[string]any{"sprite_path": p}).Revision}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler := wrapWithFileProtection("set_cel_properties", 30*time.Second, func(ctx context.Context, _ *mcp.CallToolRequest, in SetCelPropertiesInput) (*mcp.CallToolResult, *SetCelPropertiesOutput, error) {
				revision, err := aseprite.SpriteRevision(ctx, p)
				require.NoError(t, err)
				require.Equal(t, in.ExpectedRevision, revision)
				script := f.gen.SetCelProperties(in.LayerID, in.FrameNumber, in.X, nil, nil, false)
				if kind == "lua_error" {
					script += "\nerror(\"injected after save\")"
				}
				_, err = f.client.ExecuteLua(ctx, script, p)
				if err != nil {
					return nil, nil, err
				}
				if kind == "cancelled" {
					cancel()
				}
				if kind == "external_change" {
					require.NoError(t, os.WriteFile(p, append(original, 0), 0600))
				}
				return nil, &SetCelPropertiesOutput{Success: true}, nil
			}, store)
			_, _, err = handler(ctx, nil, in)
			require.Error(t, err)
			actual, err := os.ReadFile(p)
			require.NoError(t, err)
			expected := original
			if kind == "external_change" {
				expected = append(original, 0)
			}
			require.Equal(t, expected, actual)
			require.Empty(t, historyOperations(f, p))
		})
	}
}

func TestCelPropertiesSpecialLayers(t *testing.T) {
	for _, tc := range []struct{ name, setup, id string }{
		{"tilemap", `app.command.NewLayer{name="tiles",tilemap=true,gridBounds=Rectangle(0,0,8,8),ask=false};assert(app.activeLayer.isTilemap)`, "2"},
		{"reference", `app.command.NewLayer{name="ref",reference=true,ask=false};assert(app.activeLayer.isReference)`, "1"},
		{"background", `app.activeLayer=s.layers[1];app.command.BackgroundFromLayer();assert(s.layers[1].isBackground)`, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;local im=Image(1,1);im:clear(0xffffffff);s:newCel(s.layers[1],1,im);`+tc.setup+`;s:saveAs(s.filename)`)
			args := celArgs(t, f, p, tc.id, 1)
			args["x"] = 1
			rejectCel(t, f, p, args)
		})
	}
}

// A native-looking alias must not permit saveAs to pick a lossy encoder from
// the canonical target name. In particular, a single-frame PNG can preserve the
// selected cel pixels while silently discarding hidden layers and metadata.
func TestCelPropertiesCanonicalSaveFormat(t *testing.T) {
	for _, ext := range []string{".png", ".gif", ".bmp", ".ase", ".aseprite", ".ASE"} {
		t.Run(ext, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite
local im=Image(s.width,s.height);im:clear(app.pixelColor.rgba(123,45,67,255))
s:newCel(s.layers[1],1,im,Point(0,0));s.layers[1]:cel(1).opacity=128
local hidden=s:newLayer();hidden.name="keep hidden";hidden.isVisible=false
s:newCel(hidden,1,Image(im),Point(0,0));s.data="keep metadata"
s:saveAs(s.filename)`)
			canonical := filepath.Join(t.TempDir(), "actual"+ext)
			require.NoError(t, os.Rename(p, canonical))
			require.NoError(t, os.Symlink(canonical, p))
			args := celArgs(t, f, p, "1", 1)
			args["opacity"] = 255
			if ext == ".png" || ext == ".gif" || ext == ".bmp" {
				rejectCel(t, f, p, args, "Native save filename required")
			} else {
				out := f.call("set_cel_properties", args)
				require.Equal(t, true, out["success"])
				require.Len(t, historyOperations(f, p), 1)
			}
			// Both rejection and successful native save preserve all unrelated data.
			f.lua(p, `local s=app.activeSprite
assert(#s.layers==2 and #s.frames==1 and s.data=="keep metadata")
assert(s.layers[2].name=="keep hidden" and not s.layers[2].isVisible)`)
			destination, err := os.Readlink(p)
			require.NoError(t, err)
			require.Equal(t, canonical, destination)
		})
	}
}

//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// Every layer deliberately has the same hostile-looking name. Two groups have
// distinct native sharing sets with identical pixels; neither names nor pixels
// may be used as identity. A third layer has an independent equal image.
func unlinkFixture(t *testing.T, mode aseprite.ColorMode, count int, hidden bool) (*behaviorFixture, string, string) {
	t.Helper()
	f, store := newStructureFixture(t)
	p := f.sprite(mode)
	f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local root=s.layers[1]
local aGroup=s:newGroup();local bGroup=s:newGroup()
local a=s:newLayer();a.parent=aGroup;local b=s:newLayer();b.parent=bGroup
local name='same/"\n;error("injected")'
root.name=name;aGroup.name=name;bGroup.name=name;a.name=name;b.name=name
local im=Image(3,2,s.colorMode)
if s.colorMode==ColorMode.INDEXED then
 s.transparentColor=3;s.palettes[1]:setColor(0,Color{r=128,g=128,b=128,a=255});im:clear(0);im:drawPixel(2,1,3)
elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255));im:drawPixel(2,1,0)
else im:clear(app.pixelColor.rgba(128,128,128,255));im:drawPixel(2,1,0) end
s:newCel(root,1,Image(im),Point(7,8));root.isVisible=false
s:newCel(a,1,Image(im),Point(-1,2));s:newCel(b,1,Image(im),Point(4,-1))
for i=2,%d do s:newEmptyFrame() end
local frames={};for i=1,%d do frames[i]=s.frames[i] end
for _,l in ipairs({a,b}) do
 app.range.layers={l};app.range.frames=frames;app.command.LinkCels()
 l:cel(1).opacity=123;l:cel(1).data="cel-data";l:cel(1).color=Color{r=12,g=34,b=56,a=255}
 l:cel(1).properties("artist").nested={zero=0,yes=true,text="keep"}
 l:cel(1).properties.defaultValue=7
 l.data="layer-data";l.properties("artist").text="layer"
 for i=1,%d do l:cel(i).zIndex=i-2 end
end
s:newEmptyFrame();s.frames[2].duration=0.25
s.data="sprite-data";s.properties("artist").text="sprite"
local tag=s:newTag(1,%d);tag.name="tag";tag.data="tag-data";tag.repeats=3
bGroup.isEditable=false;bGroup.isVisible=false;aGroup.isVisible=%t
s:saveAs(s.filename)`, count, count, count, count, !hidden))
	return f, p, store
}

// Independent render bytes and metadata are read in separate Aseprite batches.
const unlinkState = `local s=app.activeSprite;local a=s.layers[2].layers[1];local b=s.layers[3].layers[1]
local frames={}
for i=1,#s.frames do local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,i);frames[i]=string.gsub(im.bytes,".",function(c)return string.format("%02x",string.byte(c))end) end
local rows={}
for _,l in ipairs({s.layers[1],a,b}) do for _,c in ipairs(l.cels) do
 table.insert(rows,{x=c.position.x,y=c.position.y,opacity=c.opacity,z=c.zIndex,data=c.data,color=c.color.rgbaPixel,defaultValue=c.properties.defaultValue,artist=c.properties("artist").nested})
end end
local palette={};for i=0,#s.palettes[1]-1 do table.insert(palette,s.palettes[1]:getColor(i).rgbaPixel) end
assert(s.data=="sprite-data" and s.properties("artist").text=="sprite")
assert(a.data=="layer-data" and a.properties("artist").text=="layer")
assert(s.tags[1].name=="tag" and s.tags[1].data=="tag-data" and s.tags[1].repeats==3)
assert(math.abs(s.frames[2].duration-0.25)<0.001 and not s.layers[3].isEditable)
print(json.encode({frames=frames,rows=rows,palette=palette,mode=s.colorMode,mask=s.transparentColor}))`

func rejectUnlink(t *testing.T, f *behaviorFixture, p string, args map[string]any, reason string) {
	t.Helper()
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)
	ops := historyOperations(f, p)
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "unlink_cel", Arguments: args})
	require.True(t, err != nil || r.IsError, "expected rejection: %+v", r)
	if reason != "" {
		require.NoError(t, err)
		require.Contains(t, r.Content[0].(*mcp.TextContent).Text, reason)
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

func TestUnlinkCelSavedModesAndRecovery(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		for _, count := range []int{2, 3} {
			for _, frame := range []int{1, count} {
				t.Run(fmt.Sprintf("%s/%d/frame%d", mode, count, frame), func(t *testing.T) {
					f, p, _ := unlinkFixture(t, mode, count, frame == count)
					original, err := os.ReadFile(p)
					require.NoError(t, err)
					before := f.lua(p, unlinkState)
					state := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "2/1", "frame_start": frame, "frame_end": frame})
					require.Equal(t, count, state.Layers[0].Cels[0].LinkedCelCount)
					args := map[string]any{"sprite_path": p, "layer_id": "2/1", "frame_number": frame, "expected_revision": state.Revision}
					snapshot := f.call("create_snapshot", map[string]any{"sprite_path": p})
					out := f.call("unlink_cel", args)
					require.Equal(t, true, out["success"])
					require.Len(t, out["remaining_linked_cels"], count-1)
					require.Equal(t, map[string]any{"layer_id": "2/1", "frame_number": float64(frame)}, out["cel"])
					require.NotEqual(t, state.Revision, out["revision"])
					require.Equal(t, out["revision"], readStructure(t, f, map[string]any{"sprite_path": p}).Revision)
					require.JSONEq(t, before, f.lua(p, unlinkState), "render, palette and metadata must not change")
					f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local a=s.layers[2].layers[1];local b=s.layers[3].layers[1];local selected=a:cel(%d);local peer
for i=1,%d do
 assert(b:cel(i).image==b:cel(1).image)
 assert(selected.image~=b:cel(i).image and selected.image~=s.layers[1]:cel(1).image)
 assert(a:cel(i).image.bytes==selected.image.bytes)
 if i~=%d then assert(a:cel(i).image~=selected.image);if peer then assert(peer==a:cel(i).image) else peer=a:cel(i).image end end
end`, frame, count, frame))
					rejectUnlink(t, f, p, args, "stale sprite revision")
					args["expected_revision"] = out["revision"]
					rejectUnlink(t, f, p, args, "Cel is already independent")
					ops := historyOperations(f, p)
					require.Len(t, ops, 1)
					op := ops[0].(map[string]any)
					require.Equal(t, "unlink_cel", op["tool"])
					f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]})
					restored, err := os.ReadFile(p)
					require.NoError(t, err)
					require.Equal(t, original, restored)
					f.lua(p, `assert(app.activeSprite.layers[2].layers[1]:cel(1).image==app.activeSprite.layers[2].layers[1]:cel(2).image)`)
					require.JSONEq(t, before, f.lua(p, unlinkState))
					f.call("unlink_cel", celArgs(t, f, p, "2/1", frame))
					// Restore through the actual snapshot tool, independently of operation undo.
					snap := snapshot["snapshot"].(map[string]any)
					f.call("restore_snapshot", map[string]any{"sprite_path": p, "snapshot_id": snap["snapshot_id"]})
					restored, err = os.ReadFile(p)
					require.NoError(t, err)
					require.Equal(t, original, restored)
					require.Equal(t, count, readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "2/1"}).Layers[0].Cels[0].LinkedCelCount)
					require.JSONEq(t, before, f.lua(p, unlinkState))
				})
			}
		}
	}
}

func TestUnlinkCelRejections(t *testing.T) {
	f, p, _ := unlinkFixture(t, aseprite.ColorModeRGB, 3, false)
	for _, patch := range []map[string]any{
		{"frame_number": 0}, {"frame_number": -1}, {"frame_number": 65536}, {"frame_number": 1.5}, {"frame_number": "1"}, {"frame_number": nil},
		{"layer_id": "01"}, {"layer_id": "same"}, {"layer_id": "1\";error('injected')"}, {"layer_id": nil},
		{"expected_revision": ""}, {"expected_revision": strings.Repeat("z", 64)}, {"expected_revision": nil},
	} {
		args := celArgs(t, f, p, "2/1", 1)
		for k, v := range patch {
			args[k] = v
		}
		rejectUnlink(t, f, p, args, "")
	}
	for _, key := range []string{"sprite_path", "layer_id", "frame_number", "expected_revision"} {
		args := celArgs(t, f, p, "2/1", 1)
		delete(args, key)
		rejectUnlink(t, f, p, args, "")
	}
	for _, tc := range []struct {
		id     string
		frame  int
		reason string
	}{
		{"99", 1, "Layer not found"}, {"2", 1, "Only ordinary raster"}, {"2/1", 4, "Cel does not exist"}, {"2/1", 5, "Frame out of range"}, {"3/1", 1, "Layer or ancestor is locked"}, {"1", 1, "already independent"},
	} {
		rejectUnlink(t, f, p, celArgs(t, f, p, tc.id, tc.frame), tc.reason)
	}
	f.lua(p, `local s=app.activeSprite;s.layers[2].layers[1].isEditable=false;s:saveAs(s.filename)`)
	rejectUnlink(t, f, p, celArgs(t, f, p, "2/1", 1), "Layer or ancestor is locked")
	// Each change invalidates an otherwise plausible structural address.
	for _, change := range []string{`s.layers[2].stackIndex=3`, `local l=s:newLayer();l.stackIndex=1`, `s:deleteLayer(s.layers[1])`, `s:newEmptyFrame(1)`, `s:deleteFrame(1)`, `s.data="changed"`} {
		args := celArgs(t, f, p, "2/1", 1)
		f.lua(p, `local s=app.activeSprite;`+change+`;s:saveAs(s.filename)`)
		rejectUnlink(t, f, p, args, "stale sprite revision")
	}
}

func TestUnlinkCelConcurrentRevision(t *testing.T) {
	f, p, _ := unlinkFixture(t, aseprite.ColorModeRGB, 3, false)
	args := celArgs(t, f, p, "2/1", 1)
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "unlink_cel", Arguments: args})
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
}

func TestUnlinkCelCanonicalSaveFormat(t *testing.T) {
	for _, ext := range []string{".png", ".gif", ".bmp", ".ase", ".aseprite", ".ASE"} {
		t.Run(ext, func(t *testing.T) {
			f, p, _ := unlinkFixture(t, aseprite.ColorModeRGB, 2, false)
			canonical := filepath.Join(t.TempDir(), "actual"+ext)
			require.NoError(t, os.Rename(p, canonical))
			require.NoError(t, os.Symlink(canonical, p))
			args := celArgs(t, f, p, "2/1", 1)
			if ext == ".png" || ext == ".gif" || ext == ".bmp" {
				rejectUnlink(t, f, p, args, "Native save filename required")
			} else {
				f.call("unlink_cel", args)
				require.Len(t, historyOperations(f, p), 1)
			}
			target, err := os.Readlink(p)
			require.NoError(t, err)
			require.Equal(t, canonical, target)
		})
	}
}

func TestUnlinkCelFailureAfterSave(t *testing.T) {
	for _, kind := range []string{"lua_error", "cancelled", "external_change", "backup_capacity"} {
		t.Run(kind, func(t *testing.T) {
			f, p, dir := unlinkFixture(t, aseprite.ColorModeIndexed, 3, false)
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			store := aseprite.NewSnapshotStore(dir)
			if kind == "backup_capacity" {
				store.MaxCount = 1
				_, err = store.Create(context.Background(), p, "fill capacity")
				require.NoError(t, err)
			}
			in := UnlinkCelInput{SpritePath: p, LayerID: "2/1", FrameNumber: 1, ExpectedRevision: readStructure(t, f, map[string]any{"sprite_path": p}).Revision}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler := wrapWithFileProtection("unlink_cel", 30*time.Second, func(ctx context.Context, _ *mcp.CallToolRequest, in UnlinkCelInput) (*mcp.CallToolResult, *UnlinkCelOutput, error) {
				revision, err := aseprite.SpriteRevision(ctx, p)
				require.NoError(t, err)
				require.Equal(t, in.ExpectedRevision, revision)
				script := f.gen.UnlinkCel(in.LayerID, in.FrameNumber)
				if kind == "lua_error" {
					script += "\nerror(\"injected after save\")"
				}
				raw, err := f.client.ExecuteLua(ctx, script, p)
				if err != nil {
					return nil, nil, err
				}
				var out UnlinkCelOutput
				require.NoError(t, json.Unmarshal([]byte(raw), &out))
				require.True(t, out.Success)
				if kind == "cancelled" {
					cancel()
				}
				if kind == "external_change" {
					require.NoError(t, os.WriteFile(p, append(original, 0), 0600))
				}
				return nil, &out, nil
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

func TestUnlinkCelSpecialLayers(t *testing.T) {
	for _, tc := range []struct{ name, setup, id string }{
		{"tilemap", `app.command.NewLayer{name="tiles",tilemap=true,gridBounds=Rectangle(0,0,8,8),ask=false};assert(app.activeLayer.isTilemap)`, "2"},
		{"reference", `app.command.NewLayer{name="ref",reference=true,ask=false};assert(app.activeLayer.isReference)`, "1"},
		{"background", `app.activeLayer=s.layers[1];app.command.BackgroundFromLayer();assert(s.layers[1].isBackground)`, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;app.activeLayer=s.layers[1];`+tc.setup+`;s:saveAs(s.filename)`)
			rejectUnlink(t, f, p, celArgs(t, f, p, tc.id, 1), "Only ordinary raster layers")
		})
	}
}

func TestUnlinkCelSubsequentEditsStayIndependent(t *testing.T) {
	f, p, _ := unlinkFixture(t, aseprite.ColorModeRGB, 3, false)
	f.call("unlink_cel", celArgs(t, f, p, "2/1", 1))
	// Changing the detached cel must not change its former peers. Conversely,
	// editing a remaining member must still propagate within the surviving set.
	f.lua(p, `local s=app.activeSprite;local a=s.layers[2].layers[1]
local old=a:cel(2).image.bytes
local im=Image(a:cel(1).image);im:drawPixel(0,0,0xffffffff);a:cel(1).image=im
assert(a:cel(2).image.bytes==old and a:cel(3).image.bytes==old)
local detached=a:cel(1).image.bytes
im=Image(a:cel(2).image);im:drawPixel(1,0,0xff0000ff);a:cel(2).image=im
assert(a:cel(1).image.bytes==detached and a:cel(2).image==a:cel(3).image)
s:saveAs(s.filename)`)
	f.lua(p, `local a=app.activeSprite.layers[2].layers[1]
assert(a:cel(1).image:getPixel(0,0)==0xffffffff)
assert(a:cel(1).image:getPixel(1,0)~=0xff0000ff)
assert(a:cel(2).image:getPixel(1,0)==0xff0000ff and a:cel(3).image:getPixel(1,0)==0xff0000ff)`)
	args := celArgs(t, f, p, "2/1", 1)
	args["x"] = 0
	args["opacity"] = 0
	f.call("set_cel_properties", args)
	f.lua(p, `local a=app.activeSprite.layers[2].layers[1]
assert(a:cel(1).position.x==0 and a:cel(1).opacity==0)
assert(a:cel(2).position.x==-1 and a:cel(2).opacity==123 and a:cel(2).image==a:cel(3).image)`)
}

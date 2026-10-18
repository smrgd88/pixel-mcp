//go:build integration

package tools

import (
	"context"
	"crypto/sha256"
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

func readTags(t *testing.T, f *behaviorFixture, p string) GetSpriteTagsOutput {
	t.Helper()
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)
	out := f.call("get_sprite_tags", map[string]any{"sprite_path": p})
	data, err := json.Marshal(out)
	require.NoError(t, err)
	var tags GetSpriteTagsOutput
	require.NoError(t, json.Unmarshal(data, &tags))
	after, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, before, after)
	now, err := os.Stat(p)
	require.NoError(t, err)
	require.True(t, os.SameFile(stat, now))
	require.Equal(t, stat.ModTime(), now.ModTime())
	require.Equal(t, stat.Mode(), now.Mode())
	require.Equal(t, fmt.Sprintf("%x", sha256.Sum256(before)), tags.Revision)
	return tags
}
func tagArgs(t *testing.T, f *behaviorFixture, p string, id int) map[string]any {
	t.Helper()
	return map[string]any{"sprite_path": p, "tag_id": id, "expected_revision": readTags(t, f, p).Revision}
}
func rejectTag(t *testing.T, f *behaviorFixture, p string, args map[string]any, reason ...string) {
	t.Helper()
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)
	ops := historyOperations(f, p)
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_tag_properties", Arguments: args})
	require.True(t, err != nil || r.IsError, "expected rejection: %+v", r)
	if len(reason) > 0 {
		require.NoError(t, err)
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

// This independent oracle reads native cels and renders every frame after reopening.
// A tag-only edit must preserve every output pixel and all unrelated metadata.
const tagContentOracle = `local s=app.activeSprite;local a=s.layers[2].layers[1];local b=s.layers[2].layers[2]
assert(#s.frames==4 and #s.layers==2 and #s.layers[2].layers==2)
assert(a.name=="same" and b.name=="same" and s.layers[2].name=="same")
assert(a:cel(1).image==a:cel(2).image and a:cel(1).image~=b:cel(1).image)
assert(a:cel(1).image.bytes==b:cel(1).image.bytes and a:cel(1).position==Point(2,3))
assert(a:cel(2).position==Point(2,3) and a:cel(1).opacity==123 and a:cel(2).zIndex==2)
assert(b:cel(1).position==Point(-1,5) and not b.isVisible and not s.layers[2].isEditable)
assert(s.data=="sprite" and s.properties("artist").text=="keep" and a.data=="layer" and a:cel(1).data=="cel")
assert(a.properties("artist").key==7 and a:cel(1).properties("artist").key==9)
assert(math.abs(s.frames[2].duration-0.25)<0.001)
if s.colorMode==ColorMode.INDEXED then assert(s.transparentColor==3 and a:cel(1).image:getPixel(0,0)==1) end
local result={}
for frame=1,#s.frames do
 local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,frame)
 local pixels={};for y=0,s.height-1 do for x=0,s.width-1 do table.insert(pixels,im:getPixel(x,y)) end end
 result[frame]=pixels
end
print(json.encode(result))`

func TestTagsSavedModesAndUndo(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f, store := newStructureFixture(t)
			p := f.sprite(mode)
			empty := readTags(t, f, p)
			require.NotNil(t, empty.Tags)
			require.Empty(t, empty.Tags)
			_, err := os.Stat(store)
			require.True(t, os.IsNotExist(err), "query created history store")
			f.lua(p, `local s=app.activeSprite;local g=s:newGroup();g.name="same"
local a=s:newLayer();a.parent=g;a.name="same";local b=s:newLayer();b.parent=g;b.name="same";b.isVisible=false
local im=Image(2,3,s.colorMode)
if s.colorMode==ColorMode.INDEXED then s.transparentColor=3;im:clear(1)
elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255))
else im:clear(app.pixelColor.rgba(128,128,128,255)) end
s:newCel(a,1,im,Point(2,3));s:newEmptyFrame();s:newEmptyFrame();s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
a:cel(1).opacity=123;a:cel(2).zIndex=2;s:newCel(b,1,Image(im),Point(-1,5))
s.data="sprite";s.properties("artist").text="keep";a.data="layer";a:cel(1).data="cel"
a.properties("artist").key=7;a:cel(1).properties("artist").key=9;s.frames[2].duration=0.25;g.isEditable=false
local t=s:newTag(3,4);t.name="same";t.data="target";t.properties("artist").key="target";t.color=Color{r=1,g=2,b=3,a=4};t.repeats=7
local u=s:newTag(1,2);u.name="same";u.data="other";u.properties("artist").key="other";u.aniDir=AniDir.REVERSE
local v=s:newTag(2,4);v.name="";v.data="empty";v.aniDir=AniDir.PING_PONG;v.repeats=2
s:saveAs(s.filename)`)
			before, err := f.client.ExecuteLua(context.Background(), tagContentOracle, p)
			require.NoError(t, err)
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			tags := readTags(t, f, p)
			require.Equal(t, 4, tags.FrameCount)
			require.Equal(t, []SpriteTag{
				{1, "same", 1, 2, "reverse", 0, "#000000ff"},
				{2, "", 2, 4, "pingpong", 2, "#000000ff"},
				{3, "same", 3, 4, "forward", 7, "#01020304"},
			}, tags.Tags)
			_, err = os.Stat(store)
			require.True(t, os.IsNotExist(err))
			snapshot := f.call("create_snapshot", map[string]any{"sprite_path": p, "label": "before tag edit"})["snapshot"].(map[string]any)
			name := "walk/\"\\\n한글; error('injected') --"
			args := tagArgs(t, f, p, 3)
			args["name"] = name
			args["from_frame"] = 1
			args["to_frame"] = 4
			args["direction"] = "pingpong_reverse"
			args["repeats"] = 0
			out := f.call("set_tag_properties", args)
			require.Equal(t, true, out["success"])
			after := readTags(t, f, p)
			require.Equal(t, out["revision"], after.Revision)
			require.Equal(t, SpriteTag{1, name, 1, 4, "pingpong_reverse", 0, "#01020304"}, after.Tags[0])
			require.Equal(t, float64(1), out["tag"].(map[string]any)["tag_id"])
			other, emptyTag := tags.Tags[0], tags.Tags[1]
			other.TagID = 2
			emptyTag.TagID = 3
			require.Equal(t, other, after.Tags[1])
			require.Equal(t, emptyTag, after.Tags[2])
			f.lua(p, `local s=app.activeSprite;assert(s.tags[1].data=="target" and s.tags[1].properties("artist").key=="target")
assert(s.tags[2].data=="other" and s.tags[2].properties("artist").key=="other" and s.tags[2].aniDir==AniDir.REVERSE)
assert(s.tags[3].data=="empty" and s.tags[3].name=="" and s.tags[3].repeats==2)`)
			pixels, err := f.client.ExecuteLua(context.Background(), tagContentOracle, p)
			require.NoError(t, err)
			require.Equal(t, before, pixels)
			rejectTag(t, f, p, args, "stale sprite revision")
			ops := historyOperations(f, p)
			require.Len(t, ops, 1)
			op := ops[0].(map[string]any)
			require.Equal(t, "set_tag_properties", op["tool"])
			f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]})
			restored, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, original, restored)
			pixels, err = f.client.ExecuteLua(context.Background(), tagContentOracle, p)
			require.NoError(t, err)
			require.Equal(t, before, pixels)
			// Duplicate and empty existing names may be preserved by an unrelated patch.
			for _, id := range []int{3, 2} {
				a := tagArgs(t, f, p, id)
				a["repeats"] = 65535
				if id == 3 {
					a["name"] = "same"
				}
				f.call("set_tag_properties", a)
			}
			for _, direction := range []string{"reverse", "pingpong", "pingpong_reverse", "forward"} {
				a := tagArgs(t, f, p, 3)
				a["direction"] = direction
				a["name"] = nil
				a["repeats"] = nil
				f.call("set_tag_properties", a)
				require.Equal(t, direction, readTags(t, f, p).Tags[2].Direction)
				require.Equal(t, 65535, readTags(t, f, p).Tags[2].Repeats)
			}
			// Moving a range wholly right/left must not inherit setter clamping.
			a := tagArgs(t, f, p, 1)
			a["from_frame"] = 4
			a["to_frame"] = 4
			f.call("set_tag_properties", a)
			require.Equal(t, SpriteTag{3, "same", 4, 4, "reverse", 0, "#000000ff"}, readTags(t, f, p).Tags[2])
			a = tagArgs(t, f, p, 3)
			a["from_frame"] = 1
			a["to_frame"] = 1
			f.call("set_tag_properties", a)
			require.Equal(t, SpriteTag{1, "same", 1, 1, "reverse", 0, "#000000ff"}, readTags(t, f, p).Tags[0])
			f.call("restore_snapshot", map[string]any{"sprite_path": p, "snapshot_id": snapshot["snapshot_id"]})
			restored, err = os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, original, restored)
			pixels, err = f.client.ExecuteLua(context.Background(), tagContentOracle, p)
			require.NoError(t, err)
			require.Equal(t, before, pixels)
		})
	}
}

func TestTagsRejectionsSchemaStaleAndOrder(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;for i=2,4 do s:newEmptyFrame() end
local a=s:newTag(1,2);a.name="first";local b=s:newTag(2,3);b.name="second";s:saveAs(s.filename)`)
	for _, patch := range []map[string]any{
		{}, {"repeats": nil}, {"repeats": -1}, {"repeats": 65536}, {"repeats": 1.5}, {"repeats": "1"}, {"repeats": 1e30},
		{"name": ""}, {"name": " \t"}, {"name": "a\x00b"}, {"name": "second"}, {"name": strings.Repeat("x", 65536)},
		{"direction": ""}, {"direction": "FORWARD"}, {"direction": "forward"},
		{"from_frame": 0}, {"from_frame": -1}, {"from_frame": 3}, {"to_frame": 0}, {"to_frame": 5}, {"from_frame": 3, "to_frame": 2},
		{"tag_id": 0, "repeats": 1}, {"tag_id": 3, "repeats": 1}, {"tag_id": "1", "repeats": 1}, {"tag_id": 1.5, "repeats": 1},
		{"expected_revision": "", "repeats": 1}, {"expected_revision": strings.Repeat("0", 64), "repeats": 1},
	} {
		a := tagArgs(t, f, p, 1)
		for k, v := range patch {
			a[k] = v
		}
		rejectTag(t, f, p, a)
	}
	for _, field := range []string{"sprite_path", "tag_id", "expected_revision"} {
		a := tagArgs(t, f, p, 1)
		a["repeats"] = 1
		delete(a, field)
		rejectTag(t, f, p, a)
	}
	// Single endpoint updates, overlap and identical ranges follow native ordering.
	a := tagArgs(t, f, p, 1)
	a["to_frame"] = 3
	f.call("set_tag_properties", a)
	a = tagArgs(t, f, p, 1)
	a["from_frame"] = 2
	f.call("set_tag_properties", a)
	tags := readTags(t, f, p)
	require.Equal(t, "second", tags.Tags[0].Name)
	require.Equal(t, "first", tags.Tags[1].Name)
	for _, change := range []string{
		`local t=s:newTag(1,4);t.name="inserted"`,
		`s:deleteTag(s.tags[1])`,
		`s.tags[1].fromFrame=1`,
		`s:newEmptyFrame(1)`,
		`s:deleteFrame(1)`,
		`s.data="external metadata"`,
	} {
		a = tagArgs(t, f, p, 1)
		a["repeats"] = 3
		f.lua(p, `local s=app.activeSprite;`+change+`;s:saveAs(s.filename)`)
		rejectTag(t, f, p, a, "stale sprite revision")
	}
}

func TestTagsConcurrentRevision(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;s:newTag(1,1).name="tag";s:saveAs(s.filename)`)
	a := tagArgs(t, f, p, 1)
	a["repeats"] = 1
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_tag_properties", Arguments: a})
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

func TestTagsFailureAfterSave(t *testing.T) {
	for _, kind := range []string{"lua_error", "cancelled", "external_change", "backup_capacity"} {
		t.Run(kind, func(t *testing.T) {
			f, dir := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;s:newTag(1,1).name="tag";s:saveAs(s.filename)`)
			original, err := os.ReadFile(p)
			require.NoError(t, err)
			store := aseprite.NewSnapshotStore(dir)
			if kind == "backup_capacity" {
				store.MaxCount = 1
				_, err = store.Create(context.Background(), p, "capacity")
				require.NoError(t, err)
			}
			snapshotsBefore, err := store.List(context.Background(), p)
			require.NoError(t, err)
			n := 1
			in := SetTagPropertiesInput{SpritePath: p, TagID: 1, Repeats: &n, ExpectedRevision: readTags(t, f, p).Revision}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			handler := wrapWithFileProtection("set_tag_properties", 30*time.Second, func(ctx context.Context, _ *mcp.CallToolRequest, in SetTagPropertiesInput) (*mcp.CallToolResult, *SetTagPropertiesOutput, error) {
				revision, err := aseprite.SpriteRevision(ctx, p)
				require.NoError(t, err)
				require.Equal(t, in.ExpectedRevision, revision)
				script := f.gen.SetTagProperties(in.TagID, nil, nil, nil, nil, in.Repeats)
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
				return nil, &SetTagPropertiesOutput{Success: true}, nil
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
			snapshotsAfter, err := store.List(context.Background(), p)
			require.NoError(t, err)
			require.Equal(t, snapshotsBefore, snapshotsAfter)
		})
	}
}

func TestTagsCanonicalSaveAndReadOnly(t *testing.T) {
	for _, ext := range []string{".png", ".gif", ".bmp", ".ase", ".aseprite", ".ASE"} {
		t.Run(ext, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;s:newTag(1,1).name="tag";s.data="preserve";s:saveAs(s.filename)`)
			canonical := filepath.Join(t.TempDir(), "actual"+ext)
			require.NoError(t, os.Rename(p, canonical))
			require.NoError(t, os.Symlink(canonical, p))
			a := tagArgs(t, f, p, 1)
			a["repeats"] = 1
			if ext == ".png" || ext == ".gif" || ext == ".bmp" {
				rejectTag(t, f, p, a, "Native save filename required")
			} else {
				f.call("set_tag_properties", a)
			}
			f.lua(p, `assert(app.activeSprite.data=="preserve")`)
			target, err := os.Readlink(p)
			require.NoError(t, err)
			require.Equal(t, canonical, target)
		})
	}
	f, store := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	require.NoError(t, os.Chmod(p, 0400))
	defer os.Chmod(p, 0600)
	require.Empty(t, readTags(t, f, p).Tags)
	_, err := os.Stat(store)
	require.True(t, os.IsNotExist(err))
	missing := filepath.Join(t.TempDir(), "missing.aseprite")
	for _, name := range []string{"get_sprite_tags", "set_tag_properties"} {
		r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{"sprite_path": missing, "tag_id": 1, "expected_revision": strings.Repeat("0", 64), "repeats": 1}})
		require.True(t, err != nil || r.IsError, fmt.Sprint(r))
		_, err = os.Stat(missing)
		require.True(t, os.IsNotExist(err))
	}
}

func TestTagsNameBoundaryAndNumericOrder(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite
for i=2,12 do s:newEmptyFrame() end
for i=12,1,-1 do local t=s:newTag(i,i);t.name=tostring(i) end
s:saveAs(s.filename)`)
	tags := readTags(t, f, p)
	require.Len(t, tags.Tags, 12)
	for i, tag := range tags.Tags {
		require.Equal(t, i+1, tag.TagID)
		require.Equal(t, fmt.Sprint(i+1), tag.Name)
		require.Equal(t, i+1, tag.FromFrame)
	}
	name := strings.Repeat("가", 21845) // 65535 UTF-8 bytes, the native STRING limit.
	a := tagArgs(t, f, p, 10)
	a["name"] = name
	f.call("set_tag_properties", a)
	tags = readTags(t, f, p)
	require.Equal(t, name, tags.Tags[9].Name)
	require.Equal(t, "9", tags.Tags[8].Name)
	require.Equal(t, "11", tags.Tags[10].Name)
}

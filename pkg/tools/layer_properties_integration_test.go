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

func layerRequest(tool string) map[string]any {
	switch tool {
	case "set_layer_properties":
		return map[string]any{"layer_id": "1", "name": "new"}
	case "move_layer":
		return map[string]any{"layer_id": "1", "parent_id": "", "stack_index": 2}
	default:
		return map[string]any{"name": "new", "parent_id": "", "stack_index": 1}
	}
}

func layerArgs(t *testing.T, f *behaviorFixture, p string, patch map[string]any) map[string]any {
	t.Helper()
	args := map[string]any{"sprite_path": p, "expected_revision": readStructure(t, f, map[string]any{"sprite_path": p}).Revision}
	for k, v := range patch {
		args[k] = v
	}
	return args
}
func rejectLayer(t *testing.T, f *behaviorFixture, p, tool string, args map[string]any, reason ...string) {
	t.Helper()
	before, err := os.ReadFile(p)
	require.NoError(t, err)
	stat, err := os.Stat(p)
	require.NoError(t, err)
	ops := historyOperations(f, p)
	r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
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
func editLayer(t *testing.T, f *behaviorFixture, p, tool string, patch map[string]any) map[string]any {
	t.Helper()
	args := layerArgs(t, f, p, patch)
	out := f.call(tool, args)
	require.Equal(t, true, out["success"])
	require.NotEqual(t, args["expected_revision"], out["revision"])
	require.Equal(t, out["revision"], readStructure(t, f, map[string]any{"sprite_path": p}).Revision)
	return out["layer"].(map[string]any)
}

const layerFixture = `local s=app.activeSprite
s.layers[1].name="same";s.layers[1].data="root"
if s.layers[1]:cel(1) then s:deleteCel(s.layers[1]:cel(1)) end
local g=s:newGroup();g.name="same";g.data="group"
local nested=s:newGroup();nested.parent=g;nested.name="same";nested.data="nested"
local a=s:newLayer();a.parent=nested;a.name="same";a.data="paint";a.properties("artist").text="layer property"
local b=s:newLayer();b.parent=nested;b.name="same";b.data="other";b.isVisible=false
local im=Image(2,3,s.colorMode)
if s.colorMode==ColorMode.INDEXED then s.transparentColor=3;im:clear(1)
elseif s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255))
else im:clear(app.pixelColor.rgba(128,128,128,255)) end
s:newCel(a,1,im,Point(1,2));s:newEmptyFrame();s:newEmptyFrame()
app.range.layers={a};app.range.frames={s.frames[1],s.frames[2]};app.command.LinkCels()
a:cel(1).data="cel";a:cel(1).properties("artist").text="cel property";a:cel(2).zIndex=2
s:newCel(b,1,Image(im),Point(4,5))
s.data="sprite";s.properties("artist").text="sprite property";s.properties("pixel-mcp/selection").marker="keep"
s.frames[2].duration=0.25
local tag=s:newTag(1,2);tag.name="keep";tag.data="tag data"
s:saveAs(s.filename)`
const layerPreserved = `local s=app.activeSprite
local by={};local pending={};for _,l in ipairs(s.layers) do table.insert(pending,l) end
while #pending>0 do local l=table.remove(pending);by[l.data]=l;if l.isGroup then for _,c in ipairs(l.layers) do table.insert(pending,c) end end end
local a,b=by.paint,by.other
assert(a and b and by.root and by.group and by.nested)
assert(a:cel(1).image==a:cel(2).image and a:cel(1).image~=b:cel(1).image)
assert(a:cel(1).image.bytes==b:cel(1).image.bytes and a:cel(1).image.width==2 and a:cel(1).image.height==3)
assert(a:cel(1).position==Point(1,2) and a:cel(2).position==Point(1,2) and a:cel(2).zIndex==2 and not a:cel(3))
assert(a:cel(1).opacity==255 and b:cel(1).position==Point(4,5))
assert(a.properties("artist").text=="layer property" and a:cel(1).data=="cel" and a:cel(1).properties("artist").text=="cel property")
assert(s.data=="sprite" and s.properties("artist").text=="sprite property" and s.properties("pixel-mcp/selection").marker=="keep")
assert(#s.frames==3 and #s.tags==1 and s.tags[1].name=="keep" and s.tags[1].data=="tag data" and s.tags[1].fromFrame.frameNumber==1 and s.tags[1].toFrame.frameNumber==2)
assert(math.abs(s.frames[2].duration-0.25)<0.001)
if s.colorMode==ColorMode.INDEXED then assert(s.transparentColor==3 and a:cel(1).image:getPixel(0,0)==1) end`

func TestLayerEditsSavedModesAndUndo(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale, aseprite.ColorModeIndexed} {
		t.Run(string(mode), func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(mode)
			f.lua(p, layerFixture)
			// Exercise all three tools, history snapshots, and byte-exact undo separately.
			for _, tc := range []struct {
				tool  string
				patch map[string]any
			}{
				{"set_layer_properties", map[string]any{"layer_id": "2/1/1", "name": "new", "opacity": 0}},
				{"move_layer", map[string]any{"layer_id": "2/1", "parent_id": "", "stack_index": 1}},
				{"create_layer_group", map[string]any{"parent_id": "2/1", "name": "", "stack_index": 2}},
			} {
				before, err := os.ReadFile(p)
				require.NoError(t, err)
				args := layerArgs(t, f, p, tc.patch)
				f.call(tc.tool, args)
				f.lua(p, layerPreserved)
				ops := historyOperations(f, p)
				op := ops[0].(map[string]any)
				require.Equal(t, tc.tool, op["tool"])
				require.NotEmpty(t, f.call("undo_last_operation", map[string]any{"sprite_path": p, "expected_operation_id": op["operation_id"]})["backup_snapshot"])
				after, err := os.ReadFile(p)
				require.NoError(t, err)
				require.Equal(t, before, after)
			}
			// Layer opacity 0 is distinct from omission in all three color modes.
			row := editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "opacity": 0})
			require.Equal(t, float64(0), row["opacity"])
			f.lua(p, `local s=app.activeSprite;local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,1);assert(app.pixelColor.rgbaA(im:getPixel(1,2))==0)`)
			editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "opacity": 255})
			render := `local s=app.activeSprite;local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,1);assert(app.pixelColor.rgbaA(im:getPixel(1,2))==255 and app.pixelColor.rgbaA(im:getPixel(0,0))==0)`
			f.lua(p, render)
			// IDs of both source and destination are resolved BEFORE the root shifts.
			row = editLayer(t, f, p, "move_layer", map[string]any{"layer_id": "1", "parent_id": "2/1", "stack_index": 1})
			require.Equal(t, "1/1/1", row["layer_id"])
			require.Equal(t, "1/1", row["parent_id"])
			row = editLayer(t, f, p, "move_layer", map[string]any{"layer_id": "1/1/2", "parent_id": "1/1", "stack_index": 3})
			require.Equal(t, "1/1/3", row["layer_id"])
			// Move an entire nested subtree out to root; preserve descendants and links.
			row = editLayer(t, f, p, "move_layer", map[string]any{"layer_id": "1/1", "parent_id": "", "stack_index": 1})
			require.Equal(t, "1", row["layer_id"])
			f.lua(p, layerPreserved)
			f.lua(p, render)
			row = editLayer(t, f, p, "create_layer_group", map[string]any{"parent_id": "1", "name": "", "stack_index": 1})
			require.Equal(t, "1/1", row["layer_id"])
			require.Equal(t, "", row["name"])
			require.NotContains(t, row, "opacity")
			require.NotContains(t, row, "blend_mode")
			f.lua(p, layerPreserved)
			f.lua(p, render)
		})
	}
}

func TestLayerPropertiesNamesVisibilityLocksAndBlends(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, layerFixture)
	name := "same/\"\n한글\\\t\"; error('injected') --"
	row := editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "name": name, "visible": false, "editable": false})
	require.Equal(t, name, row["name"])
	require.Equal(t, false, row["visible"])
	require.Equal(t, false, row["editable"])
	for _, patch := range []map[string]any{{"name": "no"}, {"visible": true}, {"editable": true, "name": "no"}} {
		patch["layer_id"] = "2/1/1"
		rejectLayer(t, f, p, "set_layer_properties", layerArgs(t, f, p, patch))
	}
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "editable": true})
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "name": "", "visible": true})
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2", "visible": false})
	row = editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "opacity": 123})
	require.Equal(t, true, row["visible"])
	require.Equal(t, false, row["effective_visible"])
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2", "editable": false})
	s := readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "2/1/1"})
	require.True(t, s.Layers[0].Editable)
	require.False(t, s.Layers[0].EffectiveEditable)
	rejectLayer(t, f, p, "set_layer_properties", layerArgs(t, f, p, map[string]any{"layer_id": "2/1/1", "editable": true}))
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2", "editable": true})
	// Every native blend constant must survive save/reopen and match structure output.
	for _, mode := range strings.Fields(aseprite.LayerBlendModes)[1:] {
		row = editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "blend_mode": mode})
		require.Equal(t, mode, row["blend_mode"])
		s = readStructure(t, f, map[string]any{"sprite_path": p, "layer_id": "2/1/1"})
		require.Equal(t, mode, s.Layers[0].BlendMode)
	}
	editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2/1/1", "blend_mode": "normal"})
	f.lua(p, layerPreserved)
}

func TestLayerEditsRejections(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, layerFixture)
	for _, patch := range []map[string]any{
		{"layer_id": "2/1/1"}, {"layer_id": "2/1/1", "name": nil}, {"layer_id": "2/1/1", "opacity": nil},
		{"layer_id": "2/1/1", "opacity": -1}, {"layer_id": "2/1/1", "opacity": 256}, {"layer_id": "2/1/1", "opacity": 0.5},
		{"layer_id": "2/1/1", "visible": "false"}, {"layer_id": "2/1/1", "editable": 0}, {"layer_id": "2/1/1", "name": 1},
		{"layer_id": "2/1/1", "blend_mode": "src"}, {"layer_id": "2/1/1", "blend_mode": "unknown"}, {"layer_id": "2/1/1", "blend_mode": 0},
		{"layer_id": "2/1/1", "name": "a\x00b"}, {"layer_id": "2/1/1", "name": "__mcp_clipboard__"},
		{"layer_id": "2/1/1", "opacity": 255}, {"layer_id": "2", "opacity": 0}, {"layer_id": "2", "blend_mode": "normal"},
		{"layer_id": "01", "name": "x"}, {"layer_id": "9", "name": "x"}, {"layer_id": "1/1", "name": "x"},
		{"layer_id": "1", "name": "x", "expected_revision": "invalid"},
	} {
		rejectLayer(t, f, p, "set_layer_properties", layerArgs(t, f, p, patch))
	}
	for _, patch := range []map[string]any{
		{"layer_id": "2", "parent_id": "2", "stack_index": 1},
		{"layer_id": "2", "parent_id": "2/1", "stack_index": 1},
		{"layer_id": "1", "parent_id": "1", "stack_index": 1},
		{"layer_id": "1", "parent_id": "9", "stack_index": 1},
		{"layer_id": "1", "parent_id": "", "stack_index": 0},
		{"layer_id": "1", "parent_id": "", "stack_index": 3},
		{"layer_id": "1", "parent_id": "", "stack_index": 1},
		{"layer_id": "1", "parent_id": "2/1", "stack_index": 4},
		{"layer_id": "2/1/1", "parent_id": "2/1", "stack_index": 3},
		{"layer_id": "1", "parent_id": ""}, {"layer_id": "1", "stack_index": 2},
		{"layer_id": "1", "parent_id": "", "stack_index": 1.5},
	} {
		rejectLayer(t, f, p, "move_layer", layerArgs(t, f, p, patch))
	}
	for _, patch := range []map[string]any{
		{"parent_id": "1", "name": "x", "stack_index": 1}, {"parent_id": "9", "name": "x", "stack_index": 1},
		{"parent_id": "", "name": "x", "stack_index": 4}, {"parent_id": "", "name": "x", "stack_index": 0},
		{"parent_id": "", "name": "__mcp_clipboard__", "stack_index": 1}, {"parent_id": "", "name": "a\x00b", "stack_index": 1},
		{"parent_id": "", "stack_index": 1}, {"name": "x", "stack_index": 1},
	} {
		rejectLayer(t, f, p, "create_layer_group", layerArgs(t, f, p, patch))
	}
	// Locked descendants reject whole subtree movement; a hidden destination is allowed.
	f.lua(p, `local s=app.activeSprite;s.layers[2].layers[1].layers[2].isEditable=false;s:saveAs(s.filename)`)
	rejectLayer(t, f, p, "move_layer", layerArgs(t, f, p, map[string]any{"layer_id": "2", "parent_id": "", "stack_index": 1}), "Descendant is locked")
	f.lua(p, `local s=app.activeSprite;s.layers[2].isEditable=false;s:saveAs(s.filename)`)
	for _, tool := range []string{"move_layer", "create_layer_group"} {
		patch := layerRequest(tool)
		patch["parent_id"] = "2/1"
		patch["stack_index"] = 1
		rejectLayer(t, f, p, tool, layerArgs(t, f, p, patch), "Destination or ancestor is locked")
	}
	for _, tool := range []string{"set_layer_properties", "move_layer", "create_layer_group"} {
		args := layerArgs(t, f, p, layerRequest(tool))
		args["sprite_path"] = filepath.Join(t.TempDir(), "missing.aseprite")
		r, err := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
		require.True(t, err != nil || r.IsError)
	}
}

func TestLayerEditsStaleAndConcurrent(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, layerFixture)
	for _, change := range []string{
		`s.layers[1].stackIndex=2`, `local l=s:newLayer();l.name="same";l.stackIndex=1`,
		`s:deleteLayer(s.layers[#s.layers])`, `s:newEmptyFrame(1)`, `s:deleteFrame(1)`, `s.data="external change"`,
	} {
		revision := readStructure(t, f, map[string]any{"sprite_path": p}).Revision
		f.lua(p, `local s=app.activeSprite;`+change+`;s:saveAs(s.filename)`)
		for _, tool := range []string{"set_layer_properties", "move_layer", "create_layer_group"} {
			args := layerRequest(tool)
			args["sprite_path"] = p
			args["expected_revision"] = revision
			rejectLayer(t, f, p, tool, args, "stale sprite revision")
		}
	}
	args := layerArgs(t, f, p, map[string]any{"parent_id": "", "name": "new", "stack_index": 1})
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := f.session.CallTool(context.Background(), &mcp.CallToolParams{Name: "create_layer_group", Arguments: args})
			results <- e == nil && !r.IsError
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
	// Stale ID after an API insertion must never silently rename the replacement.
	delete(args, "parent_id")
	delete(args, "stack_index")
	args["layer_id"] = "1"
	rejectLayer(t, f, p, "set_layer_properties", args, "stale sprite revision")
}

func TestLayerEditsFailureAfterSave(t *testing.T) {
	for _, operation := range []string{"set", "move", "create"} {
		for _, kind := range []string{"lua_error", "cancelled", "external_change", "backup_capacity"} {
			t.Run(operation+"/"+kind, func(t *testing.T) {
				f, dir := newStructureFixture(t)
				p := f.sprite(aseprite.ColorModeRGB)
				f.lua(p, layerFixture)
				original, err := os.ReadFile(p)
				require.NoError(t, err)
				store := aseprite.NewSnapshotStore(dir)
				if kind == "backup_capacity" {
					store.MaxCount = 1
					_, err = store.Create(context.Background(), p, "fill")
					require.NoError(t, err)
				}
				tool := map[string]string{"set": "set_layer_properties", "move": "move_layer", "create": "create_layer_group"}[operation]
				in := CreateLayerGroupInput{SpritePath: p, ExpectedRevision: readStructure(t, f, map[string]any{"sprite_path": p}).Revision, Name: "changed", ParentID: "", StackIndex: 1}
				params, _ := json.Marshal(map[string]any{"layer_id": "2", "name": "changed", "parent_id": "", "stack_index": 1})
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				saved := false
				handler := wrapWithFileProtection(tool, 30*time.Second, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateLayerGroupInput) (*mcp.CallToolResult, *LayerEditOutput, error) {
					revision, e := aseprite.SpriteRevision(ctx, p)
					require.NoError(t, e)
					require.Equal(t, in.ExpectedRevision, revision)
					script := f.gen.EditLayer(operation, string(params))
					if kind == "lua_error" {
						script += "\nerror(\"injected after save\")"
					}
					_, e = f.client.ExecuteLua(ctx, script, p)
					if e != nil {
						return nil, nil, e
					}
					saved = true
					if kind == "cancelled" {
						cancel()
					}
					if kind == "external_change" {
						require.NoError(t, os.WriteFile(p, append(original, 0), 0600))
					}
					return nil, &LayerEditOutput{Success: true}, nil
				}, store)
				_, _, err = handler(ctx, nil, in)
				require.Error(t, err)
				if kind == "lua_error" {
					require.ErrorContains(t, err, "injected after save")
				} else {
					require.True(t, saved, "failure must occur after a successful native save")
					switch kind {
					case "cancelled":
						require.ErrorIs(t, err, context.Canceled)
					case "external_change":
						require.ErrorContains(t, err, "file_changed")
					case "backup_capacity":
						require.ErrorContains(t, err, "snapshot_capacity")
					}
				}
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
}

func TestLayerEditsSpecialTypesAndCanonicalPath(t *testing.T) {
	for _, tc := range []struct{ name, setup, id string }{
		{"tilemap", `app.command.NewLayer{name="tiles",tilemap=true,gridBounds=Rectangle(0,0,8,8),ask=false};assert(app.activeLayer.isTilemap)`, "2"},
		{"reference", `app.command.NewLayer{name="ref",reference=true,ask=false};assert(app.activeLayer.isReference)`, "1"},
		{"background", `app.activeLayer=s.layers[1];app.command.BackgroundFromLayer();assert(s.layers[1].isBackground)`, "1"},
		{"clipboard", `s.layers[1].name="__mcp_clipboard__"`, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, `local s=app.activeSprite;local im=Image(1,1);im:clear(0xffffffff);s:newCel(s.layers[1],1,im);`+tc.setup+`;s:saveAs(s.filename)`)
			rejectLayer(t, f, p, "set_layer_properties", layerArgs(t, f, p, map[string]any{"layer_id": tc.id, "name": "x"}))
			rejectLayer(t, f, p, "move_layer", layerArgs(t, f, p, map[string]any{"layer_id": tc.id, "parent_id": "", "stack_index": 1}))
			if tc.name == "background" {
				rejectLayer(t, f, p, "create_layer_group", layerArgs(t, f, p, map[string]any{"name": "x", "parent_id": "", "stack_index": 1}))
				editLayer(t, f, p, "create_layer_group", map[string]any{"name": "x", "parent_id": "", "stack_index": 2})
			}
		})
	}
	for _, ext := range []string{".png", ".ase", ".ASEPRITE"} {
		t.Run(ext, func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, layerFixture)
			canonical := filepath.Join(t.TempDir(), "actual"+ext)
			require.NoError(t, os.Rename(p, canonical))
			require.NoError(t, os.Symlink(canonical, p))
			for _, tc := range []struct {
				tool  string
				patch map[string]any
			}{
				{"set_layer_properties", map[string]any{"layer_id": "1", "name": "renamed"}},
				{"move_layer", map[string]any{"layer_id": "1", "parent_id": "", "stack_index": 2}},
				{"create_layer_group", map[string]any{"name": "x", "parent_id": "", "stack_index": 1}},
			} {
				if ext == ".png" {
					rejectLayer(t, f, p, tc.tool, layerArgs(t, f, p, tc.patch), "Native save filename required")
				} else {
					editLayer(t, f, p, tc.tool, tc.patch)
				}
			}
			f.lua(p, layerPreserved)
			dest, e := os.Readlink(p)
			require.NoError(t, e)
			require.Equal(t, canonical, dest)
		})
	}
}

func TestLayerBlendRendering(t *testing.T) {
	for _, mode := range []aseprite.ColorMode{aseprite.ColorModeRGB, aseprite.ColorModeGrayscale} {
		t.Run(string(mode), func(t *testing.T) {
			f, _ := newStructureFixture(t)
			p := f.sprite(mode)
			f.lua(p, `local s=app.activeSprite;local im=Image(1,1,s.colorMode);if s.colorMode==ColorMode.GRAY then im:clear(app.pixelColor.graya(128,255)) else im:clear(app.pixelColor.rgba(128,128,128,255)) end
s:newCel(s.layers[1],1,im);local l=s:newLayer();s:newCel(l,1,Image(im));s:saveAs(s.filename)`)
			for _, tc := range []struct {
				mode string
				want int
			}{{"multiply", 64}, {"screen", 192}, {"normal", 128}} {
				editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2", "blend_mode": tc.mode})
				f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,1);local px=im:getPixel(0,0);assert(app.pixelColor.rgbaR(px)==%d and app.pixelColor.rgbaA(px)==255)`, tc.want))
			}
		})
	}
}

// Enumerate every nontrivial reorder of three same-named siblings. The expected
// permutation uses metadata identities, independently of the production ID scan.
func TestLayerMovesOrderInvariants(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local g=s:newGroup();g.isVisible=false
s.layers[1].parent=g
for i=1,2 do local l=s:newLayer();l.parent=g end
for i,l in ipairs(g.layers) do l.name="same";l.data=tostring(i) end
s:saveAs(s.filename)`)
	order := []string{"1", "2", "3"}
	for source := 1; source <= 3; source++ {
		for dest := 1; dest <= 3; dest++ {
			patch := map[string]any{"layer_id": fmt.Sprintf("1/%d", source), "parent_id": "1", "stack_index": dest}
			if source == dest {
				rejectLayer(t, f, p, "move_layer", layerArgs(t, f, p, patch))
				continue
			}
			moving := order[source-1]
			next := append([]string{}, order[:source-1]...)
			next = append(next, order[source:]...)
			next = append(next, "")
			copy(next[dest:], next[dest-1:])
			next[dest-1] = moving
			order = next
			row := editLayer(t, f, p, "move_layer", patch)
			require.Equal(t, fmt.Sprintf("1/%d", dest), row["layer_id"])
			require.Equal(t, true, row["visible"])
			require.Equal(t, false, row["effective_visible"])
			f.lua(p, fmt.Sprintf(`local s=app.activeSprite;assert(#s.layers==1 and #s.layers[1].layers==3 and #s.frames==1)
local expected={"%s","%s","%s"};for i,l in ipairs(s.layers[1].layers) do assert(l.data==expected[i] and l.name=="same") end`, order[0], order[1], order[2]))
		}
	}
}

func TestLayerPropertiesIndexedBlends(t *testing.T) {
	f, _ := newStructureFixture(t)
	p := f.sprite(aseprite.ColorModeIndexed)
	f.lua(p, `local s=app.activeSprite;s.palettes[1]:setColor(1,Color{r=128,g=128,b=128,a=255});s.transparentColor=3
local im=Image(1,1,ColorMode.INDEXED);im:clear(1);s:newCel(s.layers[1],1,im)
local l=s:newLayer();s:newCel(l,1,Image(im));s:saveAs(s.filename)`)
	for _, tc := range []struct {
		blend string
		want  int
	}{{"multiply", 64}, {"screen", 192}, {"normal", 128}} {
		blend := tc.blend
		row := editLayer(t, f, p, "set_layer_properties", map[string]any{"layer_id": "2", "blend_mode": blend})
		require.Equal(t, blend, row["blend_mode"])
		// RGB rendering blends the indexed source without modifying source indices
		// or palette entries.
		f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,1)
local px=im:getPixel(0,0);assert(app.pixelColor.rgbaR(px)==%d and app.pixelColor.rgbaA(px)==255)
assert(s.layers[2]:cel(1).image:getPixel(0,0)==1 and s.palettes[1]:getColor(1).red==128 and s.transparentColor==3)`, tc.want))
	}
}

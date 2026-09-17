//go:build integration

package tools

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestReviewFix_DuplicateNestedGroupsAndCelAttributes(t *testing.T) {
	// Cover source before/at/after the insertion point, not just a single-frame append.
	for _, sourceFrame := range []int{1, 3} {
		for _, insertAfter := range []int{0, 1, 2, 3} {
			t.Run(fmt.Sprintf("source=%d/after=%d", sourceFrame, insertAfter), func(t *testing.T) {
				f := newBehaviorFixture(t)
				p := f.sprite(aseprite.ColorModeRGB)
				f.lua(p, `local s=app.activeSprite;local l=s.layers[1];local outer=s:newGroup();outer.name="Outer";local inner=s:newGroup();inner.name="Inner";inner.parent=outer;l.parent=inner;l.name="Actor"
local c=l:cel(1);c.image:drawPixel(5,5,app.pixelColor.rgba(255,0,0,255));c.data="KEEP";c.zIndex=2;c.opacity=123;c.color=Color(40,50,60);c.properties.answer=42;c.properties("org/example").flag="extension"
s.frames[1].duration=0.173;s:newEmptyFrame(2);s:newEmptyFrame(3);s:saveAs(s.filename)`)
				f.lua(p, fmt.Sprintf(`local s=app.activeSprite;local c=s.layers[1].layers[1].layers[1]:cel(1);c.frameNumber=%d;s.frames[%d].duration=0.173;s:saveAs(s.filename)`, sourceFrame, sourceFrame))
				r := f.call("duplicate_frame", map[string]any{"sprite_path": p, "source_frame": sourceFrame, "insert_after": insertAfter})
				target := insertAfter + 1
				if insertAfter == 0 {
					target = 4
				}
				require.Equal(t, float64(target), r["new_frame_number"])
				f.lua(p, fmt.Sprintf(`local s=app.activeSprite;assert(#s.frames==4);local l=s.layers[1].layers[1].layers[1];assert(l.name=="Actor");local c=l:cel(%d);assert(c,"group cel missing");assert(c.data=="KEEP" and c.zIndex==2 and c.opacity==123);assert(c.color.red==40);assert(c.properties.answer==42 and c.properties("org/example").flag=="extension");assert(math.abs(s.frames[%d].duration-0.173)<0.001);assert(app.pixelColor.rgbaR(c.image:getPixel(5,5))==255)`, target, target))
			})
		}
	}

}

func TestReviewFix_DuplicateKeepsRenderedOrder(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	f.lua(p, `local s=app.activeSprite;local i=Image(1,1,ColorMode.RGB);i:drawPixel(0,0,app.pixelColor.rgba(255,0,0,255));local c=s:newCel(s.layers[1],1,i,Point(5,5));c.zIndex=2;local top=s:newLayer();local blue=Image(1,1,ColorMode.RGB);blue:drawPixel(0,0,app.pixelColor.rgba(0,0,255,255));s:newCel(top,1,blue,Point(5,5));s:saveAs(s.filename)`)
	f.call("duplicate_frame", map[string]any{"sprite_path": p, "source_frame": 1, "insert_after": 0})
	f.lua(p, `local s=app.activeSprite;for n=1,2 do local im=Image(16,16,ColorMode.RGB);im:drawSprite(s,n);assert(app.pixelColor.rgbaR(im:getPixel(5,5))==255,"composited color changed") end`)
}

func TestReviewFix_SelectionDoesNotRewriteUserData(t *testing.T) {
	for _, data := range []string{`{"author":"artist","empty":{},"items":[null,{},[],false,0],"null":null}`, `[null,{},[],false,0]`, `null`, `{}`, `  arbitrary text  `, `{"_pixel_mcp_original_data":"user field","large":9007199254740993}`, `{"selection":true}`, `{"selection":12}`, `{"selection":[null,{}]}`, `{"selection":{"x":1,"y":2,"w":3,"h":3,"runs":true}}`} {
		t.Run(data, func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			f.lua(p, fmt.Sprintf(`app.activeSprite.data="%s";app.activeSprite:saveAs(app.activeSprite.filename)`, aseprite.EscapeString(data)))
			for _, name := range []string{"select_all", "deselect", "select_all", "deselect"} {
				f.call(name, map[string]any{"sprite_path": p})
				f.lua(p, fmt.Sprintf(`assert(app.activeSprite.data=="%s","user data changed")`, aseprite.EscapeString(data)))
			}
		})
	}
}

func TestReviewFix_LegacyMaskDoesNotResurrectAfterDeselect(t *testing.T) {
	f := newBehaviorFixture(t)
	p := f.sprite(aseprite.ColorModeRGB)
	const legacy = `{"selection":{"x":2,"y":3,"w":3,"h":3},"unknown":{"value":null}}`
	f.lua(p, fmt.Sprintf(`app.activeSprite.data="%s";app.activeSprite:saveAs(app.activeSprite.filename)`, aseprite.EscapeString(legacy)))
	f.call("move_selection", map[string]any{"sprite_path": p, "dx": 1, "dy": 2})
	f.lua(p, `local m=app.activeSprite.properties("pixel-mcp/selection").mask;assert(m.x==3 and m.y==5)`)
	f.call("deselect", map[string]any{"sprite_path": p})
	// Selecting with add after clearing must not resurrect the legacy rectangle.
	f.call("select_rectangle", map[string]any{"sprite_path": p, "x": 10, "y": 10, "width": 1, "height": 1, "mode": "add"})
	f.lua(p, `local m=app.activeSprite.properties("pixel-mcp/selection").mask;assert(m.x==10 and m.y==10 and m.w==1 and m.h==1)`)
	f.lua(p, fmt.Sprintf(`assert(app.activeSprite.data=="%s")`, aseprite.EscapeString(legacy)))
}

func TestReviewFix_DuplicatePreservesTagInsertionSemantics(t *testing.T) {
	for _, after := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			f := newBehaviorFixture(t)
			p := f.sprite(aseprite.ColorModeRGB)
			control := f.sprite(aseprite.ColorModeRGB)
			for _, path := range []string{p, control} {
				f.lua(path, `local s=app.activeSprite;s:newEmptyFrame(2);s:newEmptyFrame(3);local tag=s:newTag(1,2);tag.name="action";s:saveAs(s.filename)`)
			}
			target := after + 1
			if after == 0 {
				target = 4
			}
			expected := f.lua(control, fmt.Sprintf(`local s=app.activeSprite;s:newEmptyFrame(%d);print(json.encode({from=s.tags[1].fromFrame.frameNumber,to=s.tags[1].toFrame.frameNumber}))`, target))
			f.call("duplicate_frame", map[string]any{"sprite_path": p, "source_frame": 3, "insert_after": after})
			actual := f.lua(p, `local s=app.activeSprite;print(json.encode({from=s.tags[1].fromFrame.frameNumber,to=s.tags[1].toFrame.frameNumber}))`)
			require.JSONEq(t, expected, actual)
		})
	}
}

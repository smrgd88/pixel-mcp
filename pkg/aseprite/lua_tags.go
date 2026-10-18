package aseprite

import (
	"fmt"
	"strconv"
)

const tagInspectionLua = `local directions={
 [AniDir.FORWARD]="forward",[AniDir.REVERSE]="reverse",
 [AniDir.PING_PONG]="pingpong",[AniDir.PING_PONG_REVERSE]="pingpong_reverse"
}
local function tagRow(t,i)
 local direction=directions[t.aniDir]
 assert(direction,"Unsupported tag direction")
 local c=t.color
 return {tag_id=i,name=t.name,from_frame=t.fromFrame.frameNumber,to_frame=t.toFrame.frameNumber,
 direction=direction,repeats=t.repeats,color=string.format("#%02x%02x%02x%02x",c.red,c.green,c.blue,c.alpha)}
end
`

// GetSpriteTags reads native tag order without saving the document.
func (g *LuaGenerator) GetSpriteTags() string {
	return tagInspectionLua + `local spr=app.activeSprite
if not spr then error("No active sprite") end
local rows={}
for i,t in ipairs(spr.tags) do rows[i]=json.encode(tagRow(t,i)) end
-- Explicit assembly preserves [] for a document without tags.
print('{"frame_count":'..#spr.frames..',"tags":['..table.concat(rows,",")..']}')`
}

// SetTagProperties edits a tag object, never a name lookup or recreated tag.
// The caller validates optional values and the bound file's revision.
func (g *LuaGenerator) SetTagProperties(id int, name *string, from, to *int, direction *string, repeats *int) string {
	number := func(v *int) string {
		if v == nil {
			return "nil"
		}
		return strconv.Itoa(*v)
	}
	str := func(v *string) string {
		if v == nil {
			return "nil"
		}
		return `"` + EscapeString(*v) + `"`
	}
	return tagInspectionLua + fmt.Sprintf(`local spr=app.activeSprite
if not spr then error("No active sprite") end
local filename=spr.filename
local saveName=filename:lower()
if not saveName:match("%%.ase$") and not saveName:match("%%.aseprite$") then error("Native save filename required") end
local tag=spr.tags[%d]
if not tag then error("Tag not found") end
local name,from,to,direction,repeats=%s,%s,%s,%s,%s
name=name or tag.name;from=from or tag.fromFrame.frameNumber;to=to or tag.toFrame.frameNumber
if from<1 or to>#spr.frames or from>to then error("Tag range out of bounds or reversed") end
local aniDir=tag.aniDir
if direction then
 local found=false
 for value,label in pairs(directions) do if label==direction then aniDir=value;found=true end end
 assert(found,"Invalid tag direction")
end
repeats=repeats or tag.repeats
if name~=tag.name then
 for _,t in ipairs(spr.tags) do if t~=tag and t.name==name then error("Duplicate tag name") end end
end
if name==tag.name and from==tag.fromFrame.frameNumber and to==tag.toFrame.frameNumber and aniDir==tag.aniDir and repeats==tag.repeats then error("No tag properties would change") end
app.transaction(function()
 if name~=tag.name then tag.name=name end
 -- Native range setters reorder tags and clamp the opposite endpoint.
 -- Keep the object reference and set both endpoints when the range changes.
 if from~=tag.fromFrame.frameNumber or to~=tag.toFrame.frameNumber then
  tag.fromFrame=from;tag.toFrame=to
 end
 if aniDir~=tag.aniDir then tag.aniDir=aniDir end
 if repeats~=tag.repeats then tag.repeats=repeats end
end)
local expected,userdata={},{}
local savedID
for i,t in ipairs(spr.tags) do
 expected[i]=tagRow(t,i);userdata[i]=t.data
 if t==tag then savedID=i end
end
assert(savedID,"Edited tag missing")
assert(spr:saveAs(filename)~=false,"Save failed")
spr:close();spr=app.open(filename)
assert(spr and #spr.tags==#expected,"Saved tags differ")
for i,t in ipairs(spr.tags) do
 local actual=tagRow(t,i)
 for k,v in pairs(expected[i]) do assert(actual[k]==v,"Saved tag properties differ") end
 assert(t.data==userdata[i],"Saved tag data differs")
end
print(json.encode({success=true,tag=tagRow(spr.tags[savedID],savedID)}))`, id, str(name), number(from), number(to), str(direction), number(repeats))
}

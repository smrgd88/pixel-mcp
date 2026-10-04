package aseprite

import (
	"fmt"
	"strconv"
)

// SetCelProperties changes only position/opacity on an existing native sharing
// set. Caller validates integer bounds and a revision of the bound source bytes.
func (g *LuaGenerator) SetCelProperties(layerID string, frame int, x, y, opacity *int, allowLinked bool) string {
	number := func(v *int) string {
		if v == nil {
			return "nil"
		}
		return strconv.Itoa(*v)
	}
	return fmt.Sprintf(`local spr=app.activeSprite
if not spr then error("No active sprite") end
local wanted,frame="%s",%d
local x,y,opacity=%s,%s,%s
local allowLinked=%t
local function scan(s)
 local all,pending={},{}
 for i=#s.layers,1,-1 do table.insert(pending,{layer=s.layers[i],id=tostring(i),editable=true}) end
 while #pending>0 do
  local n=table.remove(pending);n.editable=n.editable and n.layer.isEditable
  table.insert(all,n)
  if n.layer.isGroup then
   for i=#n.layer.layers,1,-1 do table.insert(pending,{layer=n.layer.layers[i],id=n.id.."/"..i,editable=n.editable}) end
  end
 end
 return all
end
local function eligible(n)
 local l=n.layer
 if not n.editable then error("Layer or ancestor is locked") end
 if l.isGroup or l.isTilemap or l.isReference or l.isBackground or not l.isImage then error("Only ordinary raster layers are supported") end
end
if frame>#spr.frames then error("Frame out of range") end
local nodes=scan(spr)
local target
for _,n in ipairs(nodes) do if n.id==wanted then target=n end end
if not target then error("Layer not found") end
eligible(target)
local cel=target.layer:cel(frame)
if not cel then error("Cel does not exist") end
local members={}
for _,n in ipairs(nodes) do
 for _,c in ipairs(n.layer.cels) do
  if c.image.id==cel.image.id then
   eligible(n)
   table.insert(members,{layer_id=n.id,frame_number=c.frameNumber,cel=c,z=c.zIndex})
  end
 end
end
if #members>1 and not allowLinked then error("Linked cels require allow_linked=true") end
x=x or cel.position.x;y=y or cel.position.y;opacity=opacity or cel.opacity
if x < -32768 or x > 32767 or y < -32768 or y > 32767 then error("Resulting position out of range") end
if x==cel.position.x and y==cel.position.y and opacity==cel.opacity then error("No cel properties would change") end
local pixels=cel.image.bytes
local width,height=cel.image.width,cel.image.height
app.transaction(function()
 cel.position=Point(x,y)
 cel.opacity=opacity
end)
-- Native setters share CelData. Verify the intended full set before saving.
for _,m in ipairs(members) do
 assert(m.cel.position==Point(x,y) and m.cel.opacity==opacity and m.cel.zIndex==m.z,"Unexpected linked setter result")
end
local filename=spr.filename
assert(spr:saveAs(filename)~=false,"Save failed")
spr:close()
spr=app.open(filename)
assert(spr,"Saved sprite could not be reopened")
local saved={}
for _,n in ipairs(scan(spr)) do saved[n.id]=n.layer end
local imageID
local rows={}
for _,m in ipairs(members) do
 local c=saved[m.layer_id]:cel(m.frame_number)
 assert(c and c.position==Point(x,y) and c.opacity==opacity and c.zIndex==m.z,"Saved cel properties differ")
 assert(c.image.bytes==pixels and c.image.width==width and c.image.height==height,"Saved image changed")
 if imageID then assert(c.image.id==imageID,"Native sharing lost") else imageID=c.image.id end
 table.insert(rows,{layer_id=m.layer_id,frame_number=m.frame_number,x=c.position.x,y=c.position.y,opacity=c.opacity})
end
local count=0
for _,l in pairs(saved) do for _,c in ipairs(l.cels) do if c.image.id==imageID then count=count+1 end end end
assert(count==#members,"Native sharing set changed")
print(json.encode({success=true,affected_cels=rows}))`, EscapeString(layerID), frame, number(x), number(y), number(opacity), allowLinked)
}

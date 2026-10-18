package aseprite

import "fmt"

// UnlinkCel uses Aseprite's native command to copy one cel's shared CelData.
// The caller validates the structural address and the bound file revision.
func (g *LuaGenerator) UnlinkCel(layerID string, frame int) string {
	return fmt.Sprintf(`local spr=app.activeSprite
if not spr then error("No active sprite") end
local saveName=spr.filename:lower()
if not saveName:match("%%.ase$") and not saveName:match("%%.aseprite$") then error("Native save filename required") end
local wanted,frame="%s",%d
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
if frame>#spr.frames then error("Frame out of range") end
local nodes=scan(spr)
local target
for _,n in ipairs(nodes) do if n.id==wanted then target=n end end
if not target then error("Layer not found") end
if not target.editable then error("Layer or ancestor is locked") end
local l=target.layer
if l.isGroup or l.isTilemap or l.isReference or l.isBackground or not l.isImage then error("Only ordinary raster layers are supported") end
local cel=l:cel(frame)
if not cel then error("Cel does not exist") end
local shared=cel.image.id
local before,images,peers={},{},{}
-- Capture the entire partition, including independent images with equal pixels
-- and sharing outside the requested layer/frame or any structure query page.
for _,n in ipairs(nodes) do
 for _,c in ipairs(n.layer.cels) do
  local id=c.image.id
  if not images[id] then images[id]={bytes=c.image.bytes,w=c.image.width,h=c.image.height,mode=c.image.colorMode} end
  local selected=n.id==wanted and c.frameNumber==frame
  table.insert(before,{layer_id=n.id,frame_number=c.frameNumber,partition=selected and "detached" or id,image=id,x=c.position.x,y=c.position.y,opacity=c.opacity,z=c.zIndex,data=c.data,color=c.color.rgbaPixel})
  if id==shared and not selected then table.insert(peers,{layer_id=n.id,frame_number=c.frameNumber}) end
 end
end
if #peers==0 then error("Cel is already independent") end
local function verify(s)
 local layers={}
 local count=0
 for _,n in ipairs(scan(s)) do layers[n.id]=n.layer;count=count+#n.layer.cels end
 assert(count==#before,"Cel set changed")
 local forward,reverse={},{}
 for _,b in ipairs(before) do
  local layer=layers[b.layer_id]
  local c=layer and layer:cel(b.frame_number)
  assert(c and c.position==Point(b.x,b.y) and c.opacity==b.opacity and c.zIndex==b.z and c.data==b.data and c.color.rgbaPixel==b.color,"Cel metadata changed")
  local im=images[b.image]
  assert(c.image.bytes==im.bytes and c.image.width==im.w and c.image.height==im.h and c.image.colorMode==im.mode,"Cel pixels changed")
  local id=c.image.id
  assert(not forward[b.partition] or forward[b.partition]==id,"Native sharing lost")
  assert(not reverse[id] or reverse[id]==b.partition,"Unexpected native sharing")
  forward[b.partition]=id;reverse[id]=b.partition
 end
end
app.transaction(function()
 app.activeLayer=l;app.activeFrame=spr.frames[frame]
 app.range.layers={l};app.range.frames={spr.frames[frame]}
 app.command.UnlinkCel()
 verify(spr)
end)
local filename=spr.filename
assert(spr:saveAs(filename)~=false,"Save failed")
spr:close();spr=app.open(filename)
assert(spr,"Saved sprite could not be reopened")
verify(spr)
print(json.encode({success=true,cel={layer_id=wanted,frame_number=frame},remaining_linked_cels=peers}))`, EscapeString(layerID), frame)
}

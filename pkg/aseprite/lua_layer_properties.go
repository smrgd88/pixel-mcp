package aseprite

import (
	"fmt"
	"strings"
)

// LayerBlendModes are the native file's 19 layer modes. SRC is a drawing-only
// mode, not a persistable layer mode. Names map to official BlendMode constants.
const LayerBlendModes = "normal multiply screen overlay darken lighten color_dodge color_burn hard_light soft_light difference exclusion hsl_hue hsl_saturation hsl_color hsl_luminosity addition subtract divide"

// IsLayerBlendMode reports whether name denotes a supported native layer mode.
func IsLayerBlendMode(name string) bool {
	for _, mode := range strings.Fields(LayerBlendModes) {
		if mode == name {
			return true
		}
	}
	return false
}

const layerBlendLua = `local blendNames={}
for name in ("` + LayerBlendModes + `"):gmatch("%S+") do blendNames[BlendMode[name:upper()]]=name end
`

// EditLayer uses only native setters and checks the saved hierarchy/cels after
// reopening. Caller validates input and revision under the common staging lock.
func (g *LuaGenerator) EditLayer(operation, paramsJSON string) string {
	return fmt.Sprintf("local operation=\"%s\"\nlocal p=json.decode(\"%s\")\n", EscapeString(operation), EscapeString(paramsJSON)) + layerBlendLua + `local spr=app.activeSprite
assert(spr,"No active sprite")
local filename=spr.filename
local lower=filename:lower()
assert(lower:match("%.ase$") or lower:match("%.aseprite$"),"Native save filename required")
local function scan(s)
 local nodes,byID,pending={},{},{}
 for i=#s.layers,1,-1 do table.insert(pending,{layer=s.layers[i],id=tostring(i),parent="",visible=true,editable=true}) end
 while #pending>0 do
  local n=table.remove(pending);local l=n.layer
  n.ancestorEditable=n.editable
  n.visible=n.visible and l.isVisible;n.editable=n.editable and l.isEditable
  table.insert(nodes,n);byID[n.id]=n
  if l.isGroup then for i=#l.layers,1,-1 do table.insert(pending,{layer=l.layers[i],id=n.id.."/"..i,parent=n.id,visible=n.visible,editable=n.editable}) end end
 end
 return nodes,byID
end
local function ordinary(l)
 assert(not l.isTilemap and not l.isReference and not l.isBackground and (l.isGroup or l.isImage),"Only ordinary raster layers and groups are supported")
 assert(l.name~="__mcp_clipboard__","Internal clipboard layer is protected")
end
local nodes,byID=scan(spr)
local target=byID[p.layer_id]
local destination=spr
if operation=="move" or operation=="create" then
 if p.parent_id~="" then
  local n=byID[p.parent_id];assert(n and n.layer.isGroup,"Destination group not found")
  ordinary(n.layer);assert(n.editable,"Destination or ancestor is locked");destination=n.layer
 end
end
local layer
if operation~="create" then
 assert(target,"Layer not found");layer=target.layer;ordinary(layer)
 assert(target.ancestorEditable,"Ancestor is locked")
end
if operation=="set" then
 if not layer.isEditable then
  assert(p.editable==true and p.name==nil and p.visible==nil and p.opacity==nil and p.blend_mode==nil,"Locked layer accepts only editable=true; unlock first")
 end
 assert(not layer.isGroup or (p.opacity==nil and p.blend_mode==nil),"Group opacity and blend mode are unsupported")
 if p.name~=nil then assert(p.name~="__mcp_clipboard__","Internal clipboard name is reserved") end
 local blend=p.blend_mode and BlendMode[p.blend_mode:upper()] or nil
 local changed=(p.name~=nil and p.name~=layer.name) or (p.visible~=nil and p.visible~=layer.isVisible) or (p.editable~=nil and p.editable~=layer.isEditable) or (p.opacity~=nil and p.opacity~=layer.opacity) or (blend~=nil and blend~=layer.blendMode)
 assert(changed,"No layer properties would change")
 app.transaction(function()
  if p.name~=nil then layer.name=p.name end
  if p.visible~=nil then layer.isVisible=p.visible end
  if p.opacity~=nil then layer.opacity=p.opacity end
  if blend~=nil then layer.blendMode=blend end
  if p.editable~=nil then layer.isEditable=p.editable end
 end)
elseif operation=="move" then
 assert(target.editable,"Layer or ancestor is locked")
 assert(p.parent_id~=p.layer_id and p.parent_id:sub(1,#p.layer_id+1)~=p.layer_id.."/","Group cycle or self-parent")
 for _,n in ipairs(nodes) do
  if n.id:sub(1,#p.layer_id+1)==p.layer_id.."/" then ordinary(n.layer);assert(n.editable,"Descendant is locked") end
 end
 local same=layer.parent==destination
 local count=#destination.layers+(same and 0 or 1)
 assert(p.stack_index<=count,"stack_index out of range")
 assert(not same or layer.stackIndex~=p.stack_index,"No layer properties would change")
 -- Native backgrounds must remain at the bottom of the root stack.
 assert(not (destination==spr and spr.layers[1].isBackground and p.stack_index==1),"Cannot move below background")
 app.transaction(function()
  if not same then layer.parent=destination end
  layer.stackIndex=p.stack_index
 end)
elseif operation=="create" then
 assert(p.name~="__mcp_clipboard__","Internal clipboard name is reserved")
 assert(p.stack_index<=#destination.layers+1,"stack_index out of range")
 assert(not (destination==spr and spr.layers[1].isBackground and p.stack_index==1),"Cannot insert below background")
 app.transaction(function()
  layer=spr:newGroup();layer.name=p.name
  if layer.parent~=destination then layer.parent=destination end
  layer.stackIndex=p.stack_index
 end)
else error("Unsupported layer operation") end
-- Do not report success if a native setter ignored/clamped an intended value.
if operation=="set" then
 assert(p.name==nil or layer.name==p.name,"Name setter differs")
 assert(p.visible==nil or layer.isVisible==p.visible,"Visibility setter differs")
 assert(p.editable==nil or layer.isEditable==p.editable,"Editability setter differs")
 assert(p.opacity==nil or layer.opacity==p.opacity,"Opacity setter differs")
 assert(p.blend_mode==nil or layer.blendMode==BlendMode[p.blend_mode:upper()],"Blend setter differs")
else
 assert(layer.parent==destination and layer.stackIndex==p.stack_index,"Destination or order setter differs")
 if operation=="create" then assert(layer.name==p.name and layer.isGroup and #layer.layers==0,"New group differs") end
end
local function row(n)
 local l=n.layer
 return {layer_id=n.id,parent_id=n.parent,name=l.name,kind=l.isGroup and "group" or (l.isTilemap and "tilemap" or "raster"),stack_index=l.stackIndex,
 visible=l.isVisible,editable=l.isEditable,effective_visible=n.visible,effective_editable=n.editable,opacity=l.opacity,blend_mode=blendNames[l.blendMode]}
end
-- Record expected saved state by the new structure, with canonical native image
-- sharing anchors. Image IDs themselves cannot survive reopening.
local function snapshot(s)
 local ns=scan(s);local records,images={},{}
 for _,n in ipairs(ns) do
  local l=n.layer;local r={properties=row(n),data=l.data,cels={}}
  for _,c in ipairs(l.cels) do
   local anchor=images[c.image.id] or (n.id.."@"..c.frameNumber);images[c.image.id]=anchor
   table.insert(r.cels,{frame=c.frameNumber,x=c.position.x,y=c.position.y,opacity=c.opacity,z=c.zIndex,
    width=c.image.width,height=c.image.height,bytes=c.image.bytes,anchor=anchor,data=c.data})
  end
  table.insert(records,r)
 end
 return records,ns
end
local expected,current=snapshot(spr)
assert(#current==#nodes+(operation=="create" and 1 or 0),"Unexpected layer count")
local resultID
for _,n in ipairs(current) do if n.layer==layer then resultID=n.id end end
assert(resultID,"Target lost")
local frameCount=#spr.frames
assert(spr:saveAs(filename)~=false,"Save failed")
spr:close();spr=app.open(filename);assert(spr,"Saved sprite could not be reopened")
local actual,saved=snapshot(spr)
assert(#spr.frames==frameCount and #actual==#expected,"Saved structure differs")
for i,r in ipairs(expected) do
 local a=actual[i]
 for key,value in pairs(r.properties) do assert(a.properties[key]==value,"Saved layer properties differ") end
 assert(a.data==r.data and #a.cels==#r.cels,"Saved layer differs")
 for j,c in ipairs(r.cels) do
  local ac=a.cels[j]
  for key,value in pairs(c) do assert(ac[key]==value,"Saved cel or native sharing differs") end
 end
end
local result
for _,n in ipairs(saved) do if n.id==resultID then result=row(n) end end
print(json.encode({success=true,layer=result}))`
}

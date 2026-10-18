package aseprite

import "fmt"

// GetSpriteStructure reads metadata without modifying or saving the sprite.
// Arguments must be validated by the caller; frameEnd=0 selects up to 100 frames.
func (g *LuaGenerator) GetSpriteStructure(layerID string, offset, pageSize, frameStart, frameEnd int) string {
	return layerBlendLua + fmt.Sprintf(`local spr=app.activeSprite
if not spr then error("No active sprite") end
local wanted="%s"
local offset,size,first,last=%d,%d,%d,%d
if first>#spr.frames then error("frame_start out of range") end
if last==0 then last=math.min(#spr.frames,first+99) end
if last>#spr.frames then error("frame_end out of range") end
local all={}
-- Iterative depth-first traversal avoids recursion limits on nested documents.
local pending={}
for i=#spr.layers,1,-1 do
 table.insert(pending,{layer=spr.layers[i],id=tostring(i),parent="",names={},visible=true,editable=true})
end
while #pending>0 do
 local node=table.remove(pending)
 local layer=node.layer
 local names={}
 for _,name in ipairs(node.names) do table.insert(names,name) end
 table.insert(names,layer.name)
 local visible=node.visible and layer.isVisible
 local editable=node.editable and layer.isEditable
 node.names=names;node.visible=visible;node.editable=editable
 table.insert(all,node)
 if layer.isGroup then
  for i=#layer.layers,1,-1 do
   table.insert(pending,{layer=layer.layers[i],id=node.id.."/"..i,parent=node.id,names=names,visible=visible,editable=editable})
  end
 end
end
-- Image.id is used only within this process, never returned as a persistent ID.
local images={}
for _,node in ipairs(all) do
 for _,cel in ipairs(node.layer.cels) do
  local key=cel.image.id
  local ref=node.id.."@"..cel.frameNumber
  if not images[key] then images[key]={ref=ref,count=0,frame=cel.frameNumber} end
  local info=images[key]
  if cel.frameNumber<info.frame then info.ref=ref;info.frame=cel.frameNumber end
  info.count=info.count+1
 end
end
local matching={}
for _,node in ipairs(all) do
 if wanted=="" or node.id==wanted then table.insert(matching,node) end
end
if wanted~="" and #matching==0 then error("Layer not found: "..wanted) end
if offset>#matching then error("layer_offset out of range") end
local rows={}
for i=offset+1,math.min(#matching,offset+size) do
 local node=matching[i]
 local layer=node.layer
 local kind=layer.isGroup and "group" or (layer.isTilemap and "tilemap" or "raster")
 local cells={}
 if not layer.isGroup then
  for frame=first,last do
   local cel=layer:cel(frame)
   local row={frame_number=frame,exists=cel~=nil}
   if cel then
    local info=images[cel.image.id]
    row.x=cel.position.x;row.y=cel.position.y
    row.width=cel.image.width;row.height=cel.image.height
    row.opacity=cel.opacity;row.z_index=cel.zIndex
    row.image_ref=info.ref;row.linked_cel_count=info.count
   end
   table.insert(cells,json.encode(row))
  end
 end
 -- Build array envelopes explicitly: json.encode({}) would encode an object.
 local row={layer_id=node.id,parent_id=node.parent,name=layer.name,name_path=node.names,
 kind=kind,stack_index=layer.stackIndex,visible=layer.isVisible,editable=layer.isEditable,
 effective_visible=node.visible,effective_editable=node.editable,opacity=layer.opacity,blend_mode=blendNames[layer.blendMode]}
 local encoded=json.encode(row)
 table.insert(rows,encoded:sub(1,-2)..',"cels":['..table.concat(cells,",")..']}')
end
local mode="rgb"
if spr.colorMode==ColorMode.GRAY then mode="grayscale" elseif spr.colorMode==ColorMode.INDEXED then mode="indexed" end
local out={width=spr.width,height=spr.height,color_mode=mode,frame_count=#spr.frames,layer_count=#all,
 matching_layer_count=#matching,frame_start=first,frame_end=last}
if offset+size<#matching then out.next_layer_offset=offset+size end
if last<#spr.frames then out.next_frame_start=last+1 end
local encoded=json.encode(out)
print(encoded:sub(1,-2)..',"layers":['..table.concat(rows,",")..']}')`, EscapeString(layerID), offset, pageSize, frameStart, frameEnd)
}

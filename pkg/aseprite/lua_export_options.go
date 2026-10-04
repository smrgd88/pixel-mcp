package aseprite

import (
	"fmt"
	"strings"
)

// ExportSelection describes a validated selection of saved frames and layers.
// Zero frame endpoints mean omitted. A tag is an exact name, not a CLI filter.
type ExportSelection struct {
	Tag           string
	LayerID       string
	First, Last   int
	IncludeHidden bool
	Selected      bool
}

// exportSelectionLua resolves IDs without name ambiguity and alters visibility
// only in the unsaved process document. Parent blend/opacity remain in effect.
func exportSelectionLua(o ExportSelection) string {
	return fmt.Sprintf(`local s=app.activeSprite
if not s then error("No active sprite") end
local first,last=%d,%d
if first==0 then first=1 end
if last==0 then last=#s.frames end
local tagName="%s"
if tagName~="" then
 local found
 for _,tag in ipairs(s.tags) do
  if tag.name==tagName then
   if found then error("Ambiguous export tag") end
   found=tag
  end
 end
 if not found then error("Export tag not found") end
 first=found.fromFrame.frameNumber;last=found.toFrame.frameNumber
end
if first<1 or last<first or last>#s.frames then error("Export frame range out of bounds") end
local wanted="%s"
if wanted~="" then
 local nodes,pending={},{}
 for i=#s.layers,1,-1 do table.insert(pending,{layer=s.layers[i],id=tostring(i)}) end
 local found=false
 while #pending>0 do
  local n=table.remove(pending);table.insert(nodes,n)
  if n.id==wanted then found=true end
  if n.layer.isGroup then
   for i=#n.layer.layers,1,-1 do table.insert(pending,{layer=n.layer.layers[i],id=n.id.."/"..i}) end
  end
 end
 if not found then error("Export layer not found") end
 app.transaction(function()
  for _,n in ipairs(nodes) do
   local selected=n.id==wanted or n.id:sub(1,#wanted+1)==wanted.."/"
   local ancestor=wanted:sub(1,#n.id+1)==n.id.."/"
   if not selected and not ancestor then n.layer.isVisible=false
   elseif %t then n.layer.isVisible=true end
  end
 end)
end
`, o.First, o.Last, EscapeString(o.Tag), EscapeString(o.LayerID), o.IncludeHidden)
}

// PlanExport resolves native tags and reports the saved frame interval.
func (g *LuaGenerator) PlanExport(o ExportSelection) string {
	return exportSelectionLua(o) + `local durations={}
for i=first,last do table.insert(durations,math.floor(s.frames[i].duration*1000+0.5)) end
print(json.encode({first=first,last=last,count=#s.frames,width=s.width,height=s.height,durations=durations}))`
}

// ExportSelectedFiles renders explicit source frame numbers, preserving sequence
// naming independently of Aseprite's automatic filename numbering.
func (g *LuaGenerator) ExportSelectedFiles(paths []string, frames []int, count int, o ExportSelection, trim bool) string {
	// Keep the established GIF/save-failure path when no new render option is used.
	if !o.Selected && !trim {
		return g.ExportSpriteFiles(paths, frames, count)
	}
	var b strings.Builder
	b.WriteString(exportSelectionLua(o))
	fmt.Fprintf(&b, "if #s.frames~=%d then error(\"file_changed: sprite frame count changed\") end\nlocal nonempty=false\n", count)
	for i, p := range paths {
		fmt.Fprintf(&b, `do
local im=Image(s.width,s.height,ColorMode.RGB)
im:drawSprite(s,%d)
local bounds=im:shrinkBounds()
if not bounds.isEmpty then nonempty=true end
if %t then
 if bounds.isEmpty then im=Image(1,1,ColorMode.RGB) else im=Image(im,bounds) end
end
if not im:saveAs("%s") then error("Failed to save export frame") end
end
`, frames[i], trim, EscapeString(p))
	}
	b.WriteString(`if not nonempty then error("Export selection is empty") end
print("Exported successfully")`)
	return b.String()
}

// ExportSelectedSheet uses the native layout/trim/extrude implementation, with
// explicit staging paths for both outputs. Range selection never uses tag-name
// matching in the exporter, which otherwise silently falls back to all frames.
func (g *LuaGenerator) ExportSelectedSheet(texture, metadata, layout string, border, shape, inner int, trim, extrude bool, o ExportSelection) string {
	return exportSelectionLua(o) + fmt.Sprintf(`local frames={}
for i=first,last do table.insert(frames,s.frames[i]) end
app.range.frames=frames
if %t then
 local nonempty=false
 for i=first,last do
  local im=Image(s.width,s.height,ColorMode.RGB);im:drawSprite(s,i)
  if not im:shrinkBounds().isEmpty then nonempty=true;break end
 end
 if not nonempty then error("Export selection is empty") end
end
app.command.ExportSpriteSheet{
 ui=false,recent=false,askOverwrite=false,
 type="%s",textureFilename="%s",dataFilename="%s",dataFormat="json",
 tag="**selected-frames**",layer="",splitLayers=false,splitTags=false,
 borderPadding=%d,shapePadding=%d,innerPadding=%d,
 trim=%t,extrude=%t,trimSprite=false,trimByGrid=false,
 ignoreEmpty=false,mergeDuplicates=false,openGenerated=false,
 listLayers=true,listTags=true,listSlices=true
}
local im=Image{fromFile="%s"}
if not im then error("Failed to reopen exported texture") end
print(json.encode({width=im.width,height=im.height}))
`, o.Selected || trim, EscapeString(layout), EscapeString(texture), EscapeString(metadata), border, shape, inner, trim, extrude, EscapeString(texture))
}

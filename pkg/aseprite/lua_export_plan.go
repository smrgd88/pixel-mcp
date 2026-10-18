package aseprite

import (
	"encoding/json"
	"fmt"
)

// ExportSample is the native source-mode render's trim rectangle and its first
// equal sample (1-based). Equality is used by native packed layout even when
// mergeDuplicates=false. No pixels leave the Aseprite process.
type ExportSample struct {
	X    int `json:"x"`
	Y    int `json:"y"`
	W    int `json:"w"`
	H    int `json:"h"`
	Same int `json:"same"`
}

const prepareSheetLua = `if s.colorMode==ColorMode.INDEXED and #s.palettes==1 and s.backgroundLayer and s.backgroundLayer.isVisible then
 app.transaction(function() app.command.ChangePixelFormat{ui=false,format="rgb"} end)
 if s.colorMode~=ColorMode.RGB then error("Failed to prepare background export") end
end
`

// MeasureExportSheet mirrors DocExporter::captureSamples/createRender, using
// canvas-sized scratch images, never a sheet bitmap. Explicit collection prevents
// Lua's small userdata accounting from retaining many native image allocations.
func (g *LuaGenerator) MeasureExportSheet(o ExportSelection, trim, packed bool) string {
	return exportSelectionLua(o) + measureSheetLua(trim, packed) + `print(json.encode(measured))`
}

func measureSheetLua(trim, packed bool) string {
	return fmt.Sprintf(`local function measureSheet()
 if s.width<1 or s.height<1 or s.width>32767 or s.height>32767 or s.width*s.height>67108864 or #s.frames>65535 then
  return {budget_error="export canvas exceeds dimension budget"}
 end
 %s
 local samples,seen={},{}
 local pixels=0
 local doTrim=%t and not (s.backgroundLayer and s.backgroundLayer.isVisible)
 for i=first,last do
  local im
  local b=Rectangle(0,0,s.width,s.height)
  if doTrim then
   im=Image(s.spec);im:clear(s.transparentColor);im:drawSprite(s,i)
   b=im:shrinkBounds(s.transparentColor)
   if b.isEmpty then b=Rectangle(0,0,1,1) end
  end
  local same=#samples+1
  if %t then
   -- Render the cropped sample again, as the native packer does. This also
   -- preserves transparent RGB bytes in native equality comparisons.
   im=nil;collectgarbage("collect")
   local spec=ImageSpec(s.spec);spec.width=b.width;spec.height=b.height
   im=Image(spec);im:clear(s.transparentColor);im:drawSprite(s,i,Point(-b.x,-b.y))
   local bytes=im.bytes
   -- Native image hashes exclude row-alignment bytes (Linux SIMD builds).
   local widthBytes=b.width*im.bytesPerPixel
   if im.rowStride~=widthBytes then
    local rows={}
    for y=0,b.height-1 do rows[#rows+1]=bytes:sub(y*im.rowStride+1,y*im.rowStride+widthBytes) end
    bytes=table.concat(rows)
   end
   local key=b.width..":"..b.height
   local group=seen[key]
   if not group then group={};seen[key]=group end
   if group[bytes] then same=group[bytes]
   else
    pixels=pixels+b.width*b.height
    if pixels>67108864 then return {budget_error="packed samples exceed planning pixel budget"} end
    group[bytes]=same
   end
  end
  samples[#samples+1]={x=b.x,y=b.y,w=b.width,h=b.height,same=same}
  im=nil;collectgarbage("collect")
 end
 return {samples=samples,version=tostring(app.version)}
end
-- Image.drawSprite reads this preference; DocExporter always uses false.
-- Restore the process preference on both success and failure; nothing is saved.
local compose=app.preferences.experimental.compose_groups
app.preferences.experimental.compose_groups=false
local ok,measured=pcall(measureSheet)
app.preferences.experimental.compose_groups=compose
if not ok then error(measured) end
collectgarbage("collect")
`, prepareSheetLua, trim, packed)
}

// ExportPlannedSheet remeasures the opened document before native allocation.
// A source changed after planning cannot use an old, smaller allocation budget.
func (g *LuaGenerator) ExportPlannedSheet(texture, metadata, layout string, border, shape, inner int, trim, extrude bool, o ExportSelection, samples []ExportSample) string {
	data, _ := json.Marshal(samples)
	guard := exportSelectionLua(o) + measureSheetLua(trim, layout == "packed") + fmt.Sprintf(`
local expected=json.decode("%s")
if measured.budget_error or #measured.samples~=#expected then return print(json.encode({plan_changed=true})) end
for i,a in ipairs(expected) do
 local b=measured.samples[i]
 if a.x~=b.x or a.y~=b.y or a.w~=b.w or a.h~=b.h or a.same~=b.same then return print(json.encode({plan_changed=true})) end
end
`, EscapeString(string(data)))
	// Native export resolves selection again; preparation is idempotent.
	return guard + g.ExportSelectedSheet(texture, metadata, layout, border, shape, inner, trim, extrude, o)
}

// HasStableSheetPacking identifies the native stable-sort packing implementation.
// Older supported versions use platform-dependent std::sort tie ordering, so
// consumers must not assume exact placements for equal-area samples.
func HasStableSheetPacking(version string) bool {
	parts, _, err := parseAsepriteVersion(version)
	if err != nil {
		return false
	}
	floor := [4]int{1, 3, 18, 2}
	for i := range parts {
		if parts[i] != floor[i] {
			return parts[i] > floor[i]
		}
	}
	return true
}

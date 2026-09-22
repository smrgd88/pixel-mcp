package aseprite

import (
	"fmt"
	"strings"
)

// ExportQuantizationSource renders the supported single-frame input without
// changing the document. Animation needs a separate palette sampling contract.
func (g *LuaGenerator) ExportQuantizationSource(path string) string {
	return fmt.Sprintf(`local s=app.activeSprite
if not s then error("No active sprite") end
if #s.frames~=1 then error("quantize_palette currently supports single-frame sprites only") end
local function checkLayers(layers)
 for _,layer in ipairs(layers) do
  if layer.isTilemap then error("quantize_palette does not support tilemap layers") end
  if layer.isGroup then checkLayers(layer.layers) end
 end
end
checkLayers(s.layers)
local mode="rgb"
if s.colorMode==ColorMode.INDEXED then mode="indexed" elseif s.colorMode==ColorMode.GRAY then mode="grayscale" end
local im=Image(s.width,s.height,ColorMode.RGB)
im:drawSprite(s,1)
im:saveAs("%s")
print(json.encode({color_mode=mode}))`, EscapeString(path))
}

// ApplyQuantizedPalette remaps each cel without flattening. Dithering, when
// requested, has already been applied by the caller. originalMode restores the
// input representation after the dither path's temporary RGB replacement.
func (g *LuaGenerator) ApplyQuantizedPalette(palette []string, originalColors int, algorithm string, convertToIndexed bool, dither bool, originalMode ...string) string {
	if len(palette) == 0 {
		return `error("No palette colors provided")`
	}
	var colors strings.Builder
	for _, hex := range palette {
		var c Color
		if err := c.FromHex(hex); err != nil {
			return `error("Invalid quantized palette color")`
		}
		fmt.Fprintf(&colors, "Color{r=%d,g=%d,b=%d,a=%d},", c.R, c.G, c.B, c.A)
	}
	mode := ""
	if len(originalMode) > 0 {
		mode = originalMode[0]
	}
	return fmt.Sprintf(`local s=app.activeSprite
if not s then error("No active sprite") end
local function modeName()
 if s.colorMode==ColorMode.INDEXED then return "indexed" end
 if s.colorMode==ColorMode.GRAY then return "grayscale" end
 return "rgb"
end
local targetMode="%s"
if targetMode=="" then targetMode=modeName() end
if %t then targetMode="indexed" end
local colors={%s}
-- An unused mask index lets opaque palette index zero remain usable. Never
-- allow a used opaque color to become the transparent index during conversion.
local transparent=255
local hasTransparent=false
for i,c in ipairs(colors) do if c.alpha==0 then transparent=i-1;hasTransparent=true;break end end
if targetMode=="indexed" and #colors==256 and not hasTransparent then
 error("Indexed quantization requires one unused index for transparency")
end
local target=ColorMode.RGB
if targetMode=="indexed" then target=ColorMode.INDEXED elseif targetMode=="grayscale" then target=ColorMode.GRAY end
local pc=app.pixelColor
local oldPalette=s.palettes[1]
local oldTransparent=s.transparentColor
local cache={}
local function nearest(r,g,b)
 local key=pc.rgba(r,g,b,255)
 if cache[key] then return cache[key] end
 local best,dist=nil,math.huge
 for i,c in ipairs(colors) do
  if c.alpha~=0 then
   local d=(r-c.red)^2+(g-c.green)^2+(b-c.blue)^2
   if d<dist then best=i;dist=d end
  end
 end
 if not best then error("No opaque palette colors") end
 cache[key]=best
 return best
end
-- Decode against the original palette before changing either mode or palette.
local remapped={}
for _,cel in ipairs(s.cels) do
 local src=cel.image
 local im=Image(ImageSpec{width=src.width,height=src.height,colorMode=target,transparentColor=transparent})
 for it in src:pixels() do
  local v=it();local r,g,b,a
  if src.colorMode==ColorMode.INDEXED then
   if v==oldTransparent and not cel.layer.isBackground then r=0;g=0;b=0;a=0
   else local c=oldPalette:getColor(v);r=c.red;g=c.green;b=c.blue;a=c.alpha end
  elseif src.colorMode==ColorMode.GRAY then
   r=pc.grayaV(v);g=r;b=r;a=pc.grayaA(v)
  else r=pc.rgbaR(v);g=pc.rgbaG(v);b=pc.rgbaB(v);a=pc.rgbaA(v) end
  local pixel
  if a==0 then
   if target==ColorMode.INDEXED then pixel=transparent
   elseif target==ColorMode.GRAY then pixel=pc.graya(0,0)
   else pixel=pc.rgba(0,0,0,0) end
  else
   local i=nearest(r,g,b);local c=colors[i]
   if target==ColorMode.INDEXED then pixel=i-1
   elseif target==ColorMode.GRAY then pixel=pc.graya(c.red,255)
   else pixel=pc.rgba(c.red,c.green,c.blue,c.alpha) end
  end
  im:drawPixel(it.x,it.y,pixel)
 end
 table.insert(remapped,{layer=cel.layer,frame=cel.frame,position=cel.position,image=im,opacity=cel.opacity,data=cel.data})
end
if s.colorMode~=target then
 local format=targetMode=="grayscale" and "gray" or targetMode
 app.command.ChangePixelFormat{ui=false,format=format}
end
local palette=s.palettes[1]
palette:resize(#colors)
for i,c in ipairs(colors) do palette:setColor(i-1,c) end
-- Palette resizing clamps the mask index, so assign it only afterwards.
if target==ColorMode.INDEXED then s.transparentColor=transparent end
for _,item in ipairs(remapped) do
 local cel=item.layer:cel(item.frame)
 if not cel then cel=s:newCel(item.layer,item.frame,item.image,item.position) else cel.image=item.image;cel.position=item.position end
 cel.opacity=item.opacity;cel.data=item.data
end
local hex={}
for _,c in ipairs(colors) do
 if c.alpha==0 then table.insert(hex,string.format("#%%02X%%02X%%02X%%02X",c.red,c.green,c.blue,c.alpha))
 else table.insert(hex,string.format("#%%02X%%02X%%02X",c.red,c.green,c.blue)) end
end
s:saveAs(s.filename)
print(json.encode({success=true,original_colors=%d,quantized_colors=#colors,color_mode=modeName(),palette=hex,algorithm_used="%s"}))`, EscapeString(mode), convertToIndexed, colors.String(), originalColors, EscapeString(algorithm))
}

// ReplaceWithImage generates a Lua script to replace sprite content with an external image.
// This flattens all layers and replaces the content with the provided image.
func (g *LuaGenerator) ReplaceWithImage(imagePath string) string {
	escapedPath := EscapeString(imagePath)

	return fmt.Sprintf(`local spr = app.activeSprite
if not spr then
	error("No active sprite")
end

-- Load external image
local newImg = Image{ fromFile = "%s" }
if not newImg then
	error("Failed to load image: %s")
end

app.transaction(function()
	-- Preserve the RGB remap until the new quantized palette is applied.
    if spr.colorMode ~= ColorMode.RGB then app.command.ChangePixelFormat{ui=false,format="rgb"} end
    -- Flatten all layers to a single layer
    spr:flatten()

	-- Get the flattened layer (should be only layer now)
	local layer = spr.layers[1]
	if not layer then
		error("No layer found after flattening")
	end

	-- Get the cel at frame 1
	local cel = layer:cel(1)
	if not cel then
		-- Create cel if it doesn't exist
		cel = spr:newCel(layer, 1)
	end

	-- Convert color mode if needed
	local finalImg = newImg
	if newImg.colorMode ~= spr.colorMode then
		finalImg = Image(newImg.width, newImg.height, spr.colorMode)
		finalImg:drawImage(newImg, Point(0, 0), 255, BlendMode.SRC)
	end

	-- Replace cel image
	local celX = cel.position.x
	local celY = cel.position.y
	spr:deleteCel(cel)
	spr:newCel(layer, 1, finalImg, Point(0, 0))
end)

spr:saveAs(spr.filename)
print("Sprite content replaced successfully")`, escapedPath, escapedPath)
}

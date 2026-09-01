package aseprite

import (
	"fmt"
	"strings"
)

// ApplyAutoShadingResult generates a Lua script to apply auto-shading results to a sprite.
//
// This script:
//  1. Loads the shaded image from a temporary file
//  2. Applies the shaded pixels to the specified layer
//  3. Adds generated colors to the palette
//  4. Returns JSON with colors added, final palette, and regions shaded count
//
// Parameters:
//   - tempImagePath: path to temporary PNG with shaded image
//   - layerName: name of layer to apply shading to
//   - frameNumber: frame number (1-based)
//   - generatedColors: array of hex colors generated during shading
//   - regionsShadedCount: number of regions that were shaded
//
// The script imports the shaded image and applies it to the specified layer/frame.
func (g *LuaGenerator) ApplyAutoShadingResult(tempImagePath string, layerName string, frameNumber int, generatedColors []string, regionsShadedCount int) string {
	// Build color list for palette addition
	colorList := "{\n"
	for i, hexColor := range generatedColors {
		hexColor = strings.TrimPrefix(hexColor, "#")
		if len(hexColor) != 6 {
			continue
		}

		var r, g, b int
		_, _ = fmt.Sscanf(hexColor[:2], "%x", &r)
		_, _ = fmt.Sscanf(hexColor[2:4], "%x", &g)
		_, _ = fmt.Sscanf(hexColor[4:6], "%x", &b)

		colorList += fmt.Sprintf("\t\tColor{r=%d, g=%d, b=%d, a=255}", r, g, b)
		if i < len(generatedColors)-1 {
			colorList += ","
		}
		colorList += "\n"
	}
	colorList += "\t}"

	return fmt.Sprintf(`local spr = app.activeSprite
if not spr then
	error("No active sprite")
end

-- Find target layer
local targetLayer = nil
for _, layer in ipairs(spr.layers) do
	if layer.name == %q then
		targetLayer = layer
		break
	end
end

if not targetLayer then
	error("Layer not found: " .. %q)
end

-- Load shaded image
local shadedImg = Image{fromFile=%q}
if not shadedImg then
	error("Failed to load shaded image")
end

-- Get target cel
local cel = targetLayer:cel(%d)
if not cel then
	error("No cel found at frame " .. %d)
end

local palette = spr.palettes[1]
if not palette then
	error("No palette found")
end

local generatedColors = %s
local colorsAdded = 0
local paletteCapacityExhausted = (#palette >= 256)

local function colorsEqual(a, b)
	return a.red == b.red and a.green == b.green and a.blue == b.blue and a.alpha == b.alpha
end

local function findExactPaletteIndex(color)
	for i = 0, #palette - 1 do
		if (spr.colorMode ~= ColorMode.INDEXED or i ~= spr.transparentColor) and colorsEqual(palette:getColor(i), color) then
			return i
		end
	end
	return nil
end

local function addGeneratedColor(color)
	local existingIndex = findExactPaletteIndex(color)
	if existingIndex then
		return existingIndex
	end

	local newIndex = #palette
	if spr.colorMode == ColorMode.INDEXED and newIndex == spr.transparentColor then
		newIndex = newIndex + 1
	end
	if newIndex >= 256 then
		paletteCapacityExhausted = true
		return nil
	end

	palette:resize(newIndex + 1)
	palette:setColor(newIndex, color)
	colorsAdded = colorsAdded + 1
	return newIndex
end

local function findNearestPaletteIndex(r, g, b, a, originalIndex)
	if a == 0 then
		return spr.transparentColor
	end

	local nearestIndex = nil
	local minDistance = math.huge
	for i = 0, #palette - 1 do
		if i ~= spr.transparentColor then
			local color = palette:getColor(i)
			local dr = r - color.red
			local dg = g - color.green
			local db = b - color.blue
			local da = a - color.alpha
			local distance = dr*dr + dg*dg + db*db + da*da
			if distance < minDistance then
				minDistance = distance
				nearestIndex = i
			end
		end
	end

	if paletteCapacityExhausted and minDistance > 0 and
		originalIndex ~= nil and originalIndex >= 0 and originalIndex < #palette and
		originalIndex ~= spr.transparentColor then
		return originalIndex
	end

	return nearestIndex or spr.transparentColor
end

-- Preserve every existing palette index. Add only distinct generated colors;
-- if the palette is full, non-exact shades retain their original pixel index.
for _, color in ipairs(generatedColors) do
	addGeneratedColor(color)
end

-- Create new image with shaded content
app.transaction(function()
	local finalImg = shadedImg

	if spr.colorMode == ColorMode.INDEXED then
		finalImg = Image(shadedImg.width, shadedImg.height, ColorMode.INDEXED)
		finalImg:clear(spr.transparentColor)
		local paletteIndexCache = {}
		for y = 0, shadedImg.height - 1 do
			for x = 0, shadedImg.width - 1 do
				local pixel = shadedImg:getPixel(x, y)
				local originalIndex = cel.image:getPixel(x, y)
				local cacheKey = pixel
				if paletteCapacityExhausted then
					cacheKey = tostring(pixel) .. ":" .. tostring(originalIndex)
				end
				local paletteIndex = paletteIndexCache[cacheKey]
				if paletteIndex == nil then
					local r = app.pixelColor.rgbaR(pixel)
					local g = app.pixelColor.rgbaG(pixel)
					local b = app.pixelColor.rgbaB(pixel)
					local a = app.pixelColor.rgbaA(pixel)
					paletteIndex = findNearestPaletteIndex(r, g, b, a, originalIndex)
					paletteIndexCache[cacheKey] = paletteIndex
				end
				finalImg:drawPixel(x, y, paletteIndex)
			end
		end
	elseif shadedImg.colorMode ~= spr.colorMode then
		finalImg = Image(shadedImg.width, shadedImg.height, spr.colorMode)
		finalImg:drawImage(shadedImg, Point(0, 0), 255, BlendMode.SRC)
	end

	-- Delete the old cel and create a new one with the shaded image
	local celX = cel.position.x
	local celY = cel.position.y
	spr:deleteCel(cel)
	spr:newCel(targetLayer, %d, finalImg, celX, celY)
end)

-- Build final palette for JSON output
local paletteHex = {}
for i = 0, #palette - 1 do
	local c = palette:getColor(i)
	table.insert(paletteHex, string.format("#%%02X%%02X%%02X", c.red, c.green, c.blue))
end

-- Build JSON output
local json = string.format([[{
	"success": true,
	"colors_added": %%d,
	"palette": [%%s],
	"regions_shaded": %d
}]], colorsAdded, '"' .. table.concat(paletteHex, '", "') .. '"')

-- Save sprite
spr:saveAs(spr.filename)

-- Print JSON result
print(json)`,
		EscapeString(layerName), // layer name for finding
		EscapeString(layerName), // layer name for error
		tempImagePath,           // shaded image path
		frameNumber,             // frame number for cel lookup
		frameNumber,             // frame number for error message
		colorList,               // generated colors
		frameNumber,             // frame number for newCel
		regionsShadedCount)      // regions_shaded
}

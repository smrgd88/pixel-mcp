package aseprite

import (
	"fmt"
)

// selectionStateLua shares mask persistence between batch processes. Bounds remain
// readable by older clients; runs preserve holes and non-rectangular masks.
const selectionStateLua = `local spr = app.activeSprite
if not spr then error("No active sprite") end
-- Aseprite json.decode returns JsonValue userdata, not native Lua tables.
local function plain(value)
    if type(value) ~= "userdata" and type(value) ~= "table" then return value end
    local result = {}
    for key, item in pairs(value) do result[key] = plain(item) end
    -- JsonValue arrays expose ipairs but no pairs entries.
    for key, item in ipairs(value) do result[key] = plain(item) end
    return result
end
local state = {}
if spr.data ~= "" then
    local ok, decoded = pcall(function() return plain(json.decode(spr.data)) end)
    if ok and type(decoded) == "table" and (#decoded == 0) then
        state = decoded
    else
        state = { _pixel_mcp_original_data = spr.data }
    end
end
local function restoreSelection()
    local saved = state.selection
    if spr.selection.isEmpty and type(saved) == "table" then
        local sel = Selection()
        if type(saved.runs) == "table" then
            for _, run in ipairs(saved.runs) do sel:add(Rectangle(run.x, run.y, run.w, 1)) end
        elseif saved.w and saved.h and saved.w > 0 and saved.h > 0 then
            sel = Selection(Rectangle(saved.x, saved.y, saved.w, saved.h))
        end
        spr.selection = sel
    end
end
local function persistSelection()
    if spr.selection.isEmpty then
        state.selection = nil
    else
        local bounds = spr.selection.bounds
        local saved = {x=bounds.x, y=bounds.y, w=bounds.width, h=bounds.height, runs={}}
        for y=bounds.y,bounds.y+bounds.height-1 do
            local start = nil
            for x=bounds.x,bounds.x+bounds.width do
                local inside = x < bounds.x+bounds.width and spr.selection:contains(x,y)
                if inside and not start then start = x end
                if not inside and start then
                    table.insert(saved.runs, {x=start, y=y, w=x-start})
                    start = nil
                end
            end
        end
        state.selection = saved
    end
    if not state.selection and state._pixel_mcp_original_data then
        spr.data = state._pixel_mcp_original_data
    elseif next(state) == nil then spr.data = ""
    else spr.data = json.encode(state) end
end
local function combineSelection(sel, mode)
    restoreSelection()
    if mode == "replace" then spr.selection = sel
    elseif mode == "add" then spr.selection:add(sel)
    elseif mode == "subtract" then spr.selection:subtract(sel)
    elseif mode == "intersect" then spr.selection:intersect(sel)
    else error("Invalid selection mode") end
    persistSelection()
    spr:saveAs(spr.filename)
end
local function emptyImage(width, height)
    local spec = spr.spec
    spec.width, spec.height = width, height
    local img = Image(spec)
    img:clear(spr.colorMode == ColorMode.INDEXED and spr.transparentColor or 0)
    return img
end
local function copyToClipboard(sourceCel)
    if not sourceCel then error("Source cel not found") end
    local bounds = spr.selection.bounds
    local img = emptyImage(bounds.width, bounds.height)
    for y=bounds.y,bounds.y+bounds.height-1 do
        for x=bounds.x,bounds.x+bounds.width-1 do
            local sx,sy = x-sourceCel.position.x,y-sourceCel.position.y
            if spr.selection:contains(x,y) and sx >= 0 and sy >= 0 and sx < sourceCel.image.width and sy < sourceCel.image.height then
                img:drawPixel(x-bounds.x,y-bounds.y,sourceCel.image:getPixel(sx,sy))
            end
        end
    end
    local clipboardLayer = nil
    for _, l in ipairs(spr.layers) do if l.name == "__mcp_clipboard__" then clipboardLayer = l; break end end
    if not clipboardLayer then clipboardLayer = spr:newLayer(); clipboardLayer.name = "__mcp_clipboard__" end
    clipboardLayer.isVisible = false
    local old = clipboardLayer:cel(1)
    if old then spr:deleteCel(old) end
    spr:newCel(clipboardLayer,1,img,Point(bounds.x,bounds.y))
end
`

// SelectRectangle generates a Lua script to create a rectangular selection.
//
// Creates or modifies the current selection using a rectangular region.
// Selections are used to limit drawing operations, cut/copy content, or define
// regions for transformations.
//
// Parameters:
//   - x, y: top-left corner of the selection rectangle
//   - width: rectangle width in pixels (must be positive)
//   - height: rectangle height in pixels (must be positive)
//   - mode: selection mode - "replace" (default), "add", "subtract", or "intersect"
//
// Selection modes:
//   - "replace": replaces current selection with new rectangle
//   - "add": adds rectangle to current selection (union)
//   - "subtract": removes rectangle from current selection
//   - "intersect": keeps only the intersection of current and new selection
//
// The selection is persisted to sprite.data as JSON and restored across operations.
// The sprite is saved after the selection is created.
//
// Prints "Rectangle selection created successfully" on success.
// Returns an error if no sprite is active.
func (g *LuaGenerator) SelectRectangle(x, y, width, height int, mode string) string {
	return selectionStateLua + fmt.Sprintf(`
local sel = Selection(Rectangle(%d, %d, %d, %d))
combineSelection(sel, "%s")
print("Rectangle selection created successfully")`, x, y, width, height, EscapeString(mode))
}

// SelectEllipse generates a Lua script to create an elliptical selection.
//
// Creates or modifies the current selection using an elliptical region.
// The ellipse is defined by its bounding rectangle and filled using the
// pixel-center ellipse test, constrained to the requested bounds.
//
// Parameters:
//   - x, y: top-left corner of the ellipse bounding box
//   - width: bounding box width in pixels (ellipse diameter on x-axis)
//   - height: bounding box height in pixels (ellipse diameter on y-axis)
//   - mode: selection mode - "replace" (default), "add", "subtract", or "intersect"
//
// Selection modes:
//   - "replace": replaces current selection with new ellipse
//   - "add": adds ellipse to current selection (union)
//   - "subtract": removes ellipse from current selection
//   - "intersect": keeps only the intersection of current and new selection
//
// The selection is persisted to sprite.data as JSON and restored across operations.
// The sprite is saved after the selection is created.
//
// Prints "Ellipse selection created successfully" on success.
// Returns an error if no sprite is active.
func (g *LuaGenerator) SelectEllipse(x, y, width, height int, mode string) string {
	return selectionStateLua + fmt.Sprintf(`
local sel = Selection()
local x,y,w,h = %d,%d,%d,%d
for py=y,y+h-1 do
    for px=x,x+w-1 do
        local nx,ny = (px+0.5-x-w/2)/(w/2),(py+0.5-y-h/2)/(h/2)
        if nx*nx+ny*ny <= 1 then sel:add(Rectangle(px,py,1,1)) end
    end
end
combineSelection(sel, "%s")
print("Ellipse selection created successfully")`, x, y, width, height, EscapeString(mode))
}

// SelectAll generates a Lua script to select the entire canvas.
//
// Creates a selection covering the entire sprite canvas from (0,0) to
// (sprite.width, sprite.height). This is useful before copy/cut operations
// or to quickly select all content for transformations.
//
// The selection is persisted to sprite.data as JSON and restored across operations.
// The sprite is saved after the selection is created.
//
// Prints "Select all completed successfully" on success.
// Returns an error if no sprite is active.
func (g *LuaGenerator) SelectAll() string {
	return selectionStateLua + `
spr.selection = Selection(Rectangle(0, 0, spr.width, spr.height))
persistSelection()
spr:saveAs(spr.filename)
print("Select all completed successfully")`
}

// Deselect generates a Lua script to clear the current selection.
//
// Removes the current selection mask, allowing operations to affect the entire
// canvas again. This is the opposite of SelectAll.
//
// The persisted selection state in sprite.data is cleared and the sprite is saved.
//
// Prints "Deselect completed successfully" on success.
// Returns an error if no sprite is active.
func (g *LuaGenerator) Deselect() string {
	return selectionStateLua + `
app.command.DeselectMask()
persistSelection()
spr:saveAs(spr.filename)
print("Deselect completed successfully")`
}

// MoveSelection generates a Lua script to translate the selection mask.
//
// Shifts the selection mask by the specified offset without moving the pixel
// content. This is useful for repositioning the selection after creating it,
// or for aligning selections with specific features.
//
// Parameters:
//   - dx: horizontal offset in pixels (positive = right, negative = left)
//   - dy: vertical offset in pixels (positive = down, negative = up)
//
// The selection is restored from sprite.data if needed, then moved and persisted back.
// The sprite is saved after the selection is moved.
//
// Prints "Selection moved successfully" on success.
// Returns an error if:
//   - No sprite is active
//   - No selection exists to move
func (g *LuaGenerator) MoveSelection(dx, dy int) string {
	return selectionStateLua + fmt.Sprintf(`
restoreSelection()
if spr.selection.isEmpty then error("No active selection to move") end
local bounds = spr.selection.bounds
local dx,dy = %d,%d
local moved = Selection()
for y=bounds.y,bounds.y+bounds.height-1 do
    for x=bounds.x,bounds.x+bounds.width-1 do
        if spr.selection:contains(x,y) then moved:add(Rectangle(x+dx,y+dy,1,1)) end
    end
end
spr.selection = moved
persistSelection()
spr:saveAs(spr.filename)
print("Selection moved successfully")`, dx, dy)
}

// CutSelection generates a Lua script to cut the selected pixels to clipboard.
//
// Removes the pixels within the current selection and copies them to the
// clipboard. The cut area becomes transparent (filled with transparent pixels).
//
// Parameters:
//   - layerName: name of the layer to cut from (automatically escaped for Lua safety)
//   - frameNumber: 1-based frame index to cut from
//
// The operation is wrapped in a transaction for atomicity and the sprite
// is saved after the cut is complete.
//
// Prints "Cut selection completed successfully" on success.
// Returns an error if:
//   - No sprite is active
//   - No selection exists
//   - The layer is not found
//   - The frame number is invalid
func (g *LuaGenerator) CutSelection(layerName string, frameNumber int) string {
	return selectionStateLua + fmt.Sprintf(`
restoreSelection()
if spr.selection.isEmpty then error("No active selection to cut") end
local layer = nil
for _,lyr in ipairs(spr.layers) do if lyr.name == "%s" then layer=lyr; break end end
if not layer then error("Layer not found") end
local frame = spr.frames[%d]
if not frame then error("Frame not found") end
local cel = layer:cel(frame)
if not cel then error("Source cel not found") end
app.transaction(function()
    copyToClipboard(cel)
    local bounds = spr.selection.bounds
    local clear = spr.colorMode == ColorMode.INDEXED and spr.transparentColor or 0
    for y=bounds.y,bounds.y+bounds.height-1 do
        for x=bounds.x,bounds.x+bounds.width-1 do
            local sx,sy=x-cel.position.x,y-cel.position.y
            if spr.selection:contains(x,y) and sx>=0 and sy>=0 and sx<cel.image.width and sy<cel.image.height then cel.image:drawPixel(sx,sy,clear) end
        end
    end
end)
app.command.DeselectMask()
persistSelection()
spr:saveAs(spr.filename)
print("Cut selection completed successfully")`, EscapeString(layerName), frameNumber)
}

// CopySelection generates a Lua script to copy the selected pixels to clipboard.
//
// Copies the pixels within the current selection to the clipboard without
// removing them. The clipboard content can then be pasted elsewhere.
//
// The clipboard content is stored in a hidden layer (__mcp_clipboard__) which
// persists across operations. The selection is restored from sprite.data if needed.
//
// The sprite is saved to persist the clipboard content.
//
// Prints "Copy selection completed successfully" on success.
// Returns an error if:
//   - No sprite is active
//   - No selection exists
func (g *LuaGenerator) CopySelection() string {
	return selectionStateLua + `
restoreSelection()
if spr.selection.isEmpty then error("No active selection to copy") end
-- The public copy tool has no target selector: retain first-layer/frame behavior.
local sourceCel = spr.layers[1]:cel(spr.frames[1])
app.transaction(function() copyToClipboard(sourceCel) end)
spr:saveAs(spr.filename)
print("Copy selection completed successfully")`
}

// PasteClipboard generates a Lua script to paste clipboard content.
//
// Pastes the clipboard content (from a previous copy or cut operation) to the
// specified layer and frame. The paste position can be explicitly set or will
// use the current position if not specified.
//
// Parameters:
//   - layerName: name of the layer to paste to (automatically escaped for Lua safety)
//   - frameNumber: 1-based frame index to paste to
//   - x: optional x-coordinate for paste position (nil = current position)
//   - y: optional y-coordinate for paste position (nil = current position)
//
// The clipboard content is retrieved from the hidden layer (__mcp_clipboard__).
//
// The operation is wrapped in a transaction for atomicity and the sprite
// is saved after the paste is complete.
//
// Prints "Paste completed successfully" on success.
// Returns an error if:
//   - No sprite is active
//   - The layer is not found
//   - The frame number is invalid
//   - Clipboard is empty
func (g *LuaGenerator) PasteClipboard(layerName string, frameNumber int, x, y *int) string {
	escapedName := EscapeString(layerName)

	pastePos := ""
	if x != nil && y != nil {
		pastePos = fmt.Sprintf("local pasteX, pasteY = %d, %d", *x, *y)
	} else {
		pastePos = "local pasteX, pasteY = 0, 0"
	}

	return selectionStateLua + fmt.Sprintf(`
-- Find clipboard layer
local clipboardLayer = nil
for i, lyr in ipairs(spr.layers) do
	if lyr.name == "__mcp_clipboard__" then
		clipboardLayer = lyr
		break
	end
end

if not clipboardLayer then
	error("No clipboard content available")
end

local clipCel = clipboardLayer:cel(1)
if not clipCel then
	error("No clipboard content available")
end

-- Find target layer
local layer = nil
for i, lyr in ipairs(spr.layers) do
	if lyr.name == "%s" then
		layer = lyr
		break
	end
end

if not layer then
	error("Layer not found: %s")
end

local frame = spr.frames[%d]
if not frame then
	error("Frame not found: %d")
end

-- Paste clipboard content to target layer
%s

app.transaction(function()
	local targetCel = layer:cel(frame)
    local left,top,right,bottom=pasteX,pasteY,pasteX+clipCel.image.width,pasteY+clipCel.image.height
    if targetCel then
        left,top=math.min(left,targetCel.position.x),math.min(top,targetCel.position.y)
        right,bottom=math.max(right,targetCel.position.x+targetCel.image.width),math.max(bottom,targetCel.position.y+targetCel.image.height)
    end
    local img=emptyImage(right-left,bottom-top)
    if targetCel then img:drawImage(targetCel.image,Point(targetCel.position.x-left,targetCel.position.y-top)) end
    img:drawImage(clipCel.image,Point(pasteX-left,pasteY-top))
    if targetCel then targetCel.image=img;targetCel.position=Point(left,top)
    else spr:newCel(layer,frame,img,Point(left,top)) end
end)

spr:saveAs(spr.filename)
print("Paste completed successfully")`, escapedName, escapedName, frameNumber, frameNumber, pastePos)
}

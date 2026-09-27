package aseprite

import "fmt"

// RenderReferenceImage exports the visible first frame to a private RGB image.
// The caller owns the destination and cleanup. No source save or mode conversion
// is performed, including for indexed, grayscale, grouped, or animated sources.
func (g *LuaGenerator) RenderReferenceImage(path string) string {
	return fmt.Sprintf(`local s=app.activeSprite
if not s then error("No reference sprite could be opened") end
local im=Image(s.width,s.height,ColorMode.RGB)
im:drawSprite(s,1)
if not im:saveAs("%s") then error("Failed to render reference frame") end
print("Reference frame rendered")`, EscapeString(path))
}

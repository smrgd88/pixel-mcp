package tools

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// DryRunPreview is an executed simulation, not an estimated or persistent edit.
type DryRunPreview struct {
	Before          PreviewSpriteState `json:"before" jsonschema:"State of the temporary sprite before the operation"`
	After           PreviewSpriteState `json:"after" jsonschema:"State after simulating the operation"`
	WouldChangeFile bool               `json:"would_change_file" jsonschema:"Whether the simulated file bytes changed; not a semantic pixel comparison"`
}
type PreviewSpriteState struct {
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	ColorMode   string `json:"color_mode"`
	FrameCount  int    `json:"frame_count"`
	LayerCount  int    `json:"layer_count" jsonschema:"All layer nodes including groups and hidden layers"`
	PaletteSize int    `json:"palette_size" jsonschema:"Number of entries in the first palette"`
}

type dryRunInput interface{ dryRunRequested() bool }
type dryRunOutput interface{ setDryRunPreview(*DryRunPreview) }

func (i QuantizePaletteInput) dryRunRequested() bool                { return i.DryRun }
func (i ApplyAutoShadingInput) dryRunRequested() bool               { return i.DryRun }
func (i FlattenLayersInput) dryRunRequested() bool                  { return i.DryRun }
func (o *QuantizePaletteOutput) setDryRunPreview(p *DryRunPreview)  { o.DryRun = true; o.Preview = p }
func (o *ApplyAutoShadingOutput) setDryRunPreview(p *DryRunPreview) { o.DryRun = true; o.Preview = p }
func (o *FlattenLayersOutput) setDryRunPreview(p *DryRunPreview)    { o.DryRun = true; o.Preview = p }

// This wrapper is deliberately registered only for operations whose source
// mutations all go through Client.ExecuteLua and have no persistent side outputs.
func maybeWrapWithDryRun[I dryRunInput, O dryRunOutput](name string, client *aseprite.Client, logger core.Logger, enable bool, timeout time.Duration, handler func(context.Context, *mcp.CallToolRequest, I) (*mcp.CallToolResult, O, error)) func(context.Context, *mcp.CallToolRequest, I) (*mcp.CallToolResult, O, error) {
	apply := wrapWithFileProtection(name, timeout, handler)
	wrapped := func(ctx context.Context, req *mcp.CallToolRequest, input I) (*mcp.CallToolResult, O, error) {
		if !input.dryRunRequested() {
			return apply(ctx, req, input)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		source := inputPath(input, "SpritePath")
		var result *mcp.CallToolResult
		var output O
		err := aseprite.WithSpritePreview(ctx, source, func(ctx context.Context, copyPath string) error {
			beforeHash, err := previewFileHash(copyPath)
			if err != nil {
				return err
			}
			before, err := inspectPreviewSprite(ctx, client, source)
			if err != nil {
				return err
			}
			result, output, err = handler(ctx, req, input)
			if err != nil {
				return err
			}
			if result != nil && result.IsError {
				return fmt.Errorf("preview operation returned an error")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			after, err := inspectPreviewSprite(ctx, client, source)
			if err != nil {
				return err
			}
			afterHash, err := previewFileHash(copyPath)
			if err != nil {
				return err
			}
			output.setDryRunPreview(&DryRunPreview{Before: before, After: after, WouldChangeFile: beforeHash != afterHash})
			return nil
		})
		if err != nil {
			var zero O
			return nil, zero, err
		}
		return result, output, nil
	}
	if enable {
		return wrapWithTiming(name, logger, wrapped)
	}
	return wrapped
}

func previewFileHash(path string) ([32]byte, error) {
	var sum [32]byte
	file, err := os.Open(path)
	if err != nil {
		return sum, err
	}
	defer file.Close()
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return sum, err
	}
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
func inspectPreviewSprite(ctx context.Context, client *aseprite.Client, path string) (PreviewSpriteState, error) {
	var state PreviewSpriteState
	out, err := client.ExecuteLua(ctx, `local s=app.activeSprite
if not s then error("No active sprite") end
local function countLayers(layers)
 local n=0
 for _,layer in ipairs(layers) do n=n+1;if layer.isGroup then n=n+countLayers(layer.layers) end end
 return n
end
local mode="rgb"
if s.colorMode==ColorMode.INDEXED then mode="indexed" elseif s.colorMode==ColorMode.GRAY then mode="grayscale" end
local palette=s.palettes[1]
print(json.encode({width=s.width,height=s.height,color_mode=mode,frame_count=#s.frames,layer_count=countLayers(s.layers),palette_size=palette and #palette or 0}))`, path)
	if err != nil {
		return state, err
	}
	err = parseJSON(out, &state)
	return state, err
}

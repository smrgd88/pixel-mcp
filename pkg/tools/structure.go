package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

// GetSpriteStructureInput selects a bounded page of saved layer/cel metadata.
type GetSpriteStructureInput struct {
	SpritePath  string `json:"sprite_path" jsonschema:"Saved sprite to inspect without saving or recording history"`
	LayerID     string `json:"layer_id,omitempty" jsonschema:"Exact structural layer ID such as 2/1; does not include descendants"`
	LayerOffset int    `json:"layer_offset,omitempty" jsonschema:"Zero-based offset in matching depth-first layers; default 0"`
	PageSize    int    `json:"page_size,omitempty" jsonschema:"Layer page size; default 50, range 1-100"`
	FrameStart  int    `json:"frame_start,omitempty" jsonschema:"First frame (1-based); default 1"`
	FrameEnd    int    `json:"frame_end,omitempty" jsonschema:"Last inclusive frame; default at most 100 frames from frame_start; maximum span 100"`
}

// StructureCel describes a layer/frame intersection, including absent cels.
type StructureCel struct {
	FrameNumber    int    `json:"frame_number"`
	Exists         bool   `json:"exists"`
	X              *int   `json:"x,omitempty"`
	Y              *int   `json:"y,omitempty"`
	Width          *int   `json:"width,omitempty"`
	Height         *int   `json:"height,omitempty"`
	Opacity        *int   `json:"opacity,omitempty"`
	ZIndex         *int   `json:"z_index,omitempty"`
	ImageRef       string `json:"image_ref,omitempty"`
	LinkedCelCount int    `json:"linked_cel_count,omitempty"`
}

// StructureLayer uses sibling-index paths rather than runtime memory IDs.
type StructureLayer struct {
	LayerID           string         `json:"layer_id"`
	ParentID          string         `json:"parent_id"`
	Name              string         `json:"name"`
	NamePath          []string       `json:"name_path"`
	Kind              string         `json:"kind"`
	StackIndex        int            `json:"stack_index"`
	Visible           bool           `json:"visible"`
	Editable          bool           `json:"editable"`
	EffectiveVisible  bool           `json:"effective_visible"`
	EffectiveEditable bool           `json:"effective_editable"`
	Opacity           *int           `json:"opacity,omitempty"`
	Cels              []StructureCel `json:"cels"`
}

// GetSpriteStructureOutput reports a bounded page and continuation offsets.
type GetSpriteStructureOutput struct {
	Revision           string           `json:"revision"`
	Width              int              `json:"width"`
	Height             int              `json:"height"`
	ColorMode          string           `json:"color_mode"`
	FrameCount         int              `json:"frame_count"`
	LayerCount         int              `json:"layer_count"`
	MatchingLayerCount int              `json:"matching_layer_count"`
	FrameStart         int              `json:"frame_start"`
	FrameEnd           int              `json:"frame_end"`
	NextFrameStart     int              `json:"next_frame_start,omitempty"`
	NextLayerOffset    *int             `json:"next_layer_offset,omitempty"`
	Layers             []StructureLayer `json:"layers"`
}

var structureLayerID = regexp.MustCompile(`^[1-9][0-9]*(/[1-9][0-9]*)*$`)

func validateStructureInput(in GetSpriteStructureInput) (GetSpriteStructureInput, error) {
	if in.SpritePath == "" || (in.LayerID != "" && !structureLayerID.MatchString(in.LayerID)) || in.LayerOffset < 0 || in.PageSize < 0 || in.PageSize > 100 || in.FrameStart < 0 || in.FrameEnd < 0 {
		return in, diagnostics.Errorf("invalid_arguments", "invalid sprite_path, layer_id, offset, page size or frame range")
	}
	if in.PageSize == 0 {
		in.PageSize = 50
	}
	if in.FrameStart == 0 {
		in.FrameStart = 1
	}
	if in.FrameEnd != 0 && (in.FrameEnd < in.FrameStart || in.FrameEnd-in.FrameStart >= 100) {
		return in, diagnostics.Errorf("invalid_arguments", "frame range must be ordered and contain at most 100 frames")
	}
	return in, nil
}

func registerStructureTool(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	mcp.AddTool(server, &mcp.Tool{Name: "get_sprite_structure", Description: "Read saved layer hierarchy and frame/cel existence, bounds, opacity and native image sharing. Structural layer IDs are sibling-index paths valid while hierarchy order is unchanged; set_cel_properties accepts these IDs with the returned revision precondition. Returns at most 100 layers x 100 frames, with exact layer filter and continuation offsets."}, maybeWrapConfigured("get_sprite_structure", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, in GetSpriteStructureInput) (*mcp.CallToolResult, *GetSpriteStructureOutput, error) {
		in, err := validateStructureInput(in)
		if err != nil {
			return nil, nil, err
		}
		before, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		raw, err := client.ExecuteLua(ctx, gen.GetSpriteStructure(in.LayerID, in.LayerOffset, in.PageSize, in.FrameStart, in.FrameEnd), in.SpritePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get sprite structure: %w", err)
		}
		var out GetSpriteStructureOutput
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, nil, fmt.Errorf("failed to parse sprite structure: %w", err)
		}
		after, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		if before != after {
			return nil, nil, diagnostics.Errorf("file_changed", "source changed during structure inspection")
		}
		out.Revision = before
		return nil, &out, nil
	}))
}

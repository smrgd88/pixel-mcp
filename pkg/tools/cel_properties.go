package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

// SetCelPropertiesInput targets an existing cel in an exact saved revision.
type SetCelPropertiesInput struct {
	SpritePath       string `json:"sprite_path" jsonschema:"Native .aseprite or .ase file to edit"`
	LayerID          string `json:"layer_id" jsonschema:"Exact structural ID from get_sprite_structure, such as 2/1"`
	FrameNumber      int    `json:"frame_number" jsonschema:"Existing cel frame, 1-based"`
	ExpectedRevision string `json:"expected_revision" jsonschema:"Required revision from get_sprite_structure; refresh after every edit"`
	X                *int   `json:"x,omitempty" jsonschema:"Sprite-absolute x, -32768 to 32767; omitted or null keeps current value"`
	Y                *int   `json:"y,omitempty" jsonschema:"Sprite-absolute y, -32768 to 32767; omitted or null keeps current value"`
	Opacity          *int   `json:"opacity,omitempty" jsonschema:"Cel opacity, 0 to 255; omitted or null keeps current value"`
	AllowLinked      bool   `json:"allow_linked,omitempty" jsonschema:"Explicitly permit changing ALL cels sharing the native image; default false rejects linked cels"`
}

// CelProperties reports one affected cel after saving and reopening.
type CelProperties struct {
	LayerID     string `json:"layer_id"`
	FrameNumber int    `json:"frame_number"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Opacity     int    `json:"opacity"`
}

// SetCelPropertiesOutput lists the full native sharing set affected by the edit.
type SetCelPropertiesOutput struct {
	Success      bool            `json:"success"`
	Revision     string          `json:"revision"`
	AffectedCels []CelProperties `json:"affected_cels"`
	Warnings     []ToolWarning   `json:"warnings,omitempty"`
}

var spriteRevisionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func validateCelProperties(in SetCelPropertiesInput) error {
	ext := strings.ToLower(filepath.Ext(in.SpritePath))
	if (ext != ".ase" && ext != ".aseprite") || !structureLayerID.MatchString(in.LayerID) || in.FrameNumber < 1 || in.FrameNumber > 65535 || !spriteRevisionPattern.MatchString(in.ExpectedRevision) {
		return diagnostics.Errorf("invalid_arguments", "native sprite_path, layer_id, frame_number and expected_revision are required")
	}
	if in.X == nil && in.Y == nil && in.Opacity == nil {
		return diagnostics.Errorf("invalid_arguments", "at least one cel property is required")
	}
	for _, coordinate := range []*int{in.X, in.Y} {
		if coordinate != nil && (*coordinate < -32768 || *coordinate > 32767) {
			return diagnostics.Errorf("invalid_arguments", "cel coordinates must fit native signed 16-bit fields")
		}
	}
	if in.Opacity != nil && (*in.Opacity < 0 || *in.Opacity > 255) {
		return diagnostics.Errorf("invalid_arguments", "opacity must be between 0 and 255")
	}
	return nil
}

func registerCelPropertiesTool(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	mcp.AddTool(server, &mcp.Tool{Name: "set_cel_properties", Description: "Set existing raster cel position and/or opacity using a fresh get_sprite_structure revision. Negative/off-canvas coordinates allowed within signed 16-bit range. Rejects locked ancestors, group/tilemap/reference/background layers, absent cels and unchanged requests. Hidden layers allowed. Linked cels require allow_linked=true and change as a whole; never unlinks."}, maybeWrapConfigured("set_cel_properties", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, in SetCelPropertiesInput) (*mcp.CallToolResult, *SetCelPropertiesOutput, error) {
		if err := validateCelProperties(in); err != nil {
			return nil, nil, err
		}
		// The wrapper has already acquired the source/history locks and bound a
		// private working copy. Checking that copy closes the check-to-stage gap.
		revision, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		if revision != in.ExpectedRevision {
			return nil, nil, diagnostics.Errorf("file_changed", "stale sprite revision; query structure again")
		}
		raw, err := client.ExecuteLua(ctx, gen.SetCelProperties(in.LayerID, in.FrameNumber, in.X, in.Y, in.Opacity, in.AllowLinked), in.SpritePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to set cel properties: %w", err)
		}
		var out SetCelPropertiesOutput
		if err = json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, nil, fmt.Errorf("parse cel properties: %w", err)
		}
		if !out.Success || len(out.AffectedCels) == 0 {
			return nil, nil, fmt.Errorf("missing saved cel result")
		}
		out.Revision, err = aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		if len(out.AffectedCels) > 1 {
			out.Warnings = []ToolWarning{{Code: "linked_cel_properties", Message: "Position/opacity changes affect every cel sharing this native image. See affected_cels for the complete saved result."}}
		}
		return nil, &out, nil
	}))
}

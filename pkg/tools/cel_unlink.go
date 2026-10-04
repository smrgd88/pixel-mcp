package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

// UnlinkCelInput selects one saved raster cel with an optimistic revision guard.
type UnlinkCelInput struct {
	SpritePath       string `json:"sprite_path" jsonschema:"Native .aseprite or .ase file to edit"`
	LayerID          string `json:"layer_id" jsonschema:"Exact structural ID from get_sprite_structure, such as 2/1"`
	FrameNumber      int    `json:"frame_number" jsonschema:"Existing linked cel frame, 1-based"`
	ExpectedRevision string `json:"expected_revision" jsonschema:"Required revision from get_sprite_structure; refresh after every edit"`
}

// CelReference is a structural address valid only for its saved revision.
type CelReference struct {
	LayerID     string `json:"layer_id"`
	FrameNumber int    `json:"frame_number"`
}

// UnlinkCelOutput identifies the independent cel and its former sharing peers.
type UnlinkCelOutput struct {
	Success             bool           `json:"success"`
	Revision            string         `json:"revision"`
	Cel                 CelReference   `json:"cel"`
	RemainingLinkedCels []CelReference `json:"remaining_linked_cels"`
}

func validateUnlinkCel(in UnlinkCelInput) error {
	ext := strings.ToLower(filepath.Ext(in.SpritePath))
	if (ext != ".ase" && ext != ".aseprite") || !structureLayerID.MatchString(in.LayerID) || in.FrameNumber < 1 || in.FrameNumber > 65535 || !spriteRevisionPattern.MatchString(in.ExpectedRevision) {
		return diagnostics.Errorf("invalid_arguments", "native sprite_path, layer_id, frame_number and expected_revision are required")
	}
	return nil
}

func registerUnlinkCelTool(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	mcp.AddTool(server, &mcp.Tool{Name: "unlink_cel", Description: "Detach exactly one existing raster cel from its native shared image using a fresh get_sprite_structure revision. Copies pixels and cel metadata without moving or converting them; other members retain sharing. Rejects independent/absent cels, locked ancestors and group/tilemap/reference/background targets. Hidden targets allowed. Returns the independent cel and former peers (a sole remaining peer is independent)."}, maybeWrapConfigured("unlink_cel", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, in UnlinkCelInput) (*mcp.CallToolResult, *UnlinkCelOutput, error) {
		if err := validateUnlinkCel(in); err != nil {
			return nil, nil, err
		}
		// Read the wrapper-bound staging bytes, under source/history locks.
		revision, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		if revision != in.ExpectedRevision {
			return nil, nil, diagnostics.Errorf("file_changed", "stale sprite revision; query structure again")
		}
		raw, err := client.ExecuteLua(ctx, gen.UnlinkCel(in.LayerID, in.FrameNumber), in.SpritePath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to unlink cel: %w", err)
		}
		var out UnlinkCelOutput
		if err = json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, nil, fmt.Errorf("parse unlinked cel: %w", err)
		}
		if !out.Success || out.Cel.LayerID != in.LayerID || out.Cel.FrameNumber != in.FrameNumber || len(out.RemainingLinkedCels) == 0 {
			return nil, nil, fmt.Errorf("missing saved unlink result")
		}
		out.Revision, err = aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		return nil, &out, nil
	}))
}

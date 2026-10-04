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

// SetLayerPropertiesInput edits local properties of a revision-guarded layer.
type SetLayerPropertiesInput struct {
	SpritePath       string  `json:"sprite_path" jsonschema:"Native .aseprite or .ase file"`
	ExpectedRevision string  `json:"expected_revision" jsonschema:"Fresh revision from get_sprite_structure"`
	LayerID          string  `json:"layer_id" jsonschema:"Structural sibling-index path, e.g. 2/1"`
	Name             *string `json:"name,omitempty" jsonschema:"New name; empty allowed; omitted or null keeps current value"`
	Visible          *bool   `json:"visible,omitempty" jsonschema:"Local visibility; omitted or null keeps current value"`
	Editable         *bool   `json:"editable,omitempty" jsonschema:"Local editability; false locks; omitted or null keeps current value"`
	Opacity          *int    `json:"opacity,omitempty" jsonschema:"Raster layer opacity 0-255; groups unsupported; omitted or null keeps current value"`
	BlendMode        *string `json:"blend_mode,omitempty" jsonschema:"Native layer mode: normal, multiply, screen, overlay, darken, lighten, color_dodge, color_burn, hard_light, soft_light, difference, exclusion, hsl_hue, hsl_saturation, hsl_color, hsl_luminosity, addition, subtract, divide; groups unsupported"`
}

// MoveLayerInput moves an existing subtree to an exact final sibling position.
type MoveLayerInput struct {
	SpritePath       string `json:"sprite_path" jsonschema:"Native .aseprite or .ase file"`
	ExpectedRevision string `json:"expected_revision" jsonschema:"Fresh revision from get_sprite_structure"`
	LayerID          string `json:"layer_id" jsonschema:"Source structural ID in expected_revision"`
	ParentID         string `json:"parent_id" jsonschema:"Destination group ID in expected_revision; empty string means sprite root"`
	StackIndex       int    `json:"stack_index" jsonschema:"Required final 1-based bottom-to-top index, after removing source from its old position"`
}

// CreateLayerGroupInput creates an empty group without implicitly moving layers.
type CreateLayerGroupInput struct {
	SpritePath       string `json:"sprite_path" jsonschema:"Native .aseprite or .ase file"`
	ExpectedRevision string `json:"expected_revision" jsonschema:"Fresh revision from get_sprite_structure"`
	Name             string `json:"name" jsonschema:"Group name; empty and duplicate names allowed"`
	ParentID         string `json:"parent_id" jsonschema:"Parent group ID in expected_revision; empty string means sprite root"`
	StackIndex       int    `json:"stack_index" jsonschema:"Required 1-based bottom-to-top insertion index, 1 through child count plus one"`
}

// LayerProperties describes the saved target, using its NEW structural ID.
type LayerProperties struct {
	LayerID           string `json:"layer_id"`
	ParentID          string `json:"parent_id"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	StackIndex        int    `json:"stack_index"`
	Visible           bool   `json:"visible"`
	Editable          bool   `json:"editable"`
	EffectiveVisible  bool   `json:"effective_visible"`
	EffectiveEditable bool   `json:"effective_editable"`
	Opacity           *int   `json:"opacity,omitempty"`
	BlendMode         string `json:"blend_mode,omitempty"`
}

// LayerEditOutput returns the saved revision and target; requery all other IDs.
type LayerEditOutput struct {
	Success  bool            `json:"success"`
	Revision string          `json:"revision"`
	Layer    LayerProperties `json:"layer"`
}

func validateLayerRevision(path, revision string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if (ext != ".ase" && ext != ".aseprite") || !spriteRevisionPattern.MatchString(revision) {
		return diagnostics.Errorf("invalid_arguments", "native sprite_path and expected_revision are required")
	}
	return nil
}
func validateSetLayerProperties(in SetLayerPropertiesInput) error {
	if err := validateLayerRevision(in.SpritePath, in.ExpectedRevision); err != nil {
		return err
	}
	if !structureLayerID.MatchString(in.LayerID) {
		return diagnostics.Errorf("invalid_arguments", "invalid layer_id")
	}
	if in.Name == nil && in.Visible == nil && in.Editable == nil && in.Opacity == nil && in.BlendMode == nil {
		return diagnostics.Errorf("invalid_arguments", "at least one layer property is required")
	}
	if in.Name != nil && (strings.ContainsRune(*in.Name, 0) || len(*in.Name) > 65535) {
		return diagnostics.Errorf("invalid_arguments", "name must fit native string without NUL")
	}
	if in.Opacity != nil && (*in.Opacity < 0 || *in.Opacity > 255) {
		return diagnostics.Errorf("invalid_arguments", "opacity must be between 0 and 255")
	}
	if in.BlendMode != nil && !aseprite.IsLayerBlendMode(*in.BlendMode) {
		return diagnostics.Errorf("invalid_arguments", "unsupported layer blend_mode")
	}
	return nil
}
func validateLayerDestination(path, revision, parent string, index int) error {
	if err := validateLayerRevision(path, revision); err != nil {
		return err
	}
	if (parent != "" && !structureLayerID.MatchString(parent)) || index < 1 || index > 65535 {
		return diagnostics.Errorf("invalid_arguments", "invalid parent_id or stack_index")
	}
	return nil
}

// Called only inside maybeWrapConfigured's source/history lock and staging scope.
func executeLayerEdit(ctx context.Context, client *aseprite.Client, gen *aseprite.LuaGenerator, path, revision, operation string, input any) (*mcp.CallToolResult, *LayerEditOutput, error) {
	actual, err := aseprite.SpriteRevision(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	if actual != revision {
		return nil, nil, diagnostics.Errorf("file_changed", "stale sprite revision; query structure again")
	}
	params, err := json.Marshal(input)
	if err != nil {
		return nil, nil, err
	}
	raw, err := client.ExecuteLua(ctx, gen.EditLayer(operation, string(params)), path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to edit layer: %w", err)
	}
	var out LayerEditOutput
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, nil, fmt.Errorf("parse layer edit: %w", err)
	}
	if !out.Success || !structureLayerID.MatchString(out.Layer.LayerID) {
		return nil, nil, fmt.Errorf("missing saved layer result")
	}
	out.Revision, err = aseprite.SpriteRevision(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return nil, &out, nil
}

func registerLayerPropertyTools(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	mcp.AddTool(server, &mcp.Tool{Name: "set_layer_properties", Description: "Edit name, local visibility/editability, raster opacity/blend using a fresh structure revision. Ordinary raster/group only; hidden targets allowed. Locked ancestors rejected; a locally locked target accepts only editable=true (unlock first). Group opacity/blend unsupported. No-op rejected. Requery structure after success."}, maybeWrapConfigured("set_layer_properties", logger, cfg, func(ctx context.Context, _ *mcp.CallToolRequest, in SetLayerPropertiesInput) (*mcp.CallToolResult, *LayerEditOutput, error) {
		if err := validateSetLayerProperties(in); err != nil {
			return nil, nil, err
		}
		return executeLayerEdit(ctx, client, gen, in.SpritePath, in.ExpectedRevision, "set", in)
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "move_layer", Description: "Move/reorder an ordinary raster layer or whole group subtree at a fresh revision. Both source and destination IDs refer to that revision. Required stack_index is the final bottom-to-top sibling index. Empty parent_id is root. Rejects cycles, locked source/descendants/destination, special layers and no-op. All structural IDs must be requeried after success."}, maybeWrapConfigured("move_layer", logger, cfg, func(ctx context.Context, _ *mcp.CallToolRequest, in MoveLayerInput) (*mcp.CallToolResult, *LayerEditOutput, error) {
		if err := validateLayerDestination(in.SpritePath, in.ExpectedRevision, in.ParentID, in.StackIndex); err != nil {
			return nil, nil, err
		}
		if !structureLayerID.MatchString(in.LayerID) {
			return nil, nil, diagnostics.Errorf("invalid_arguments", "invalid layer_id")
		}
		return executeLayerEdit(ctx, client, gen, in.SpritePath, in.ExpectedRevision, "move", in)
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "create_layer_group", Description: "Create an empty group at an explicit parent and sibling index using a fresh structure revision. Empty parent_id is root. Rejects locked/non-group destinations. Does not move or delete layers. Requery structure after success."}, maybeWrapConfigured("create_layer_group", logger, cfg, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateLayerGroupInput) (*mcp.CallToolResult, *LayerEditOutput, error) {
		if err := validateLayerDestination(in.SpritePath, in.ExpectedRevision, in.ParentID, in.StackIndex); err != nil {
			return nil, nil, err
		}
		if strings.ContainsRune(in.Name, 0) || len(in.Name) > 65535 {
			return nil, nil, diagnostics.Errorf("invalid_arguments", "name must fit native string without NUL")
		}
		return executeLayerEdit(ctx, client, gen, in.SpritePath, in.ExpectedRevision, "create", in)
	}))
}

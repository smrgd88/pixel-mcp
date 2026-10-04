package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

// GetSpriteTagsInput inspects the tags in a saved document.
type GetSpriteTagsInput struct {
	SpritePath string `json:"sprite_path" jsonschema:"Saved sprite to inspect without saving or recording history"`
}

// SpriteTag identifies a tag by its 1-based position in this saved revision only.
type SpriteTag struct {
	TagID     int    `json:"tag_id"`
	Name      string `json:"name"`
	FromFrame int    `json:"from_frame"`
	ToFrame   int    `json:"to_frame"`
	Direction string `json:"direction"`
	Repeats   int    `json:"repeats"`
	Color     string `json:"color"`
}

// GetSpriteTagsOutput preserves native tag order, including duplicate/empty names.
type GetSpriteTagsOutput struct {
	Revision   string      `json:"revision"`
	FrameCount int         `json:"frame_count"`
	Tags       []SpriteTag `json:"tags"`
}

// SetTagPropertiesInput patches one existing tag in an exact saved revision.
type SetTagPropertiesInput struct {
	SpritePath       string  `json:"sprite_path" jsonschema:"Native .aseprite or .ase file to edit"`
	TagID            int     `json:"tag_id" jsonschema:"1-based tag_id from get_sprite_tags; not a permanent ID"`
	ExpectedRevision string  `json:"expected_revision" jsonschema:"Required revision from get_sprite_tags; refresh after every edit"`
	Name             *string `json:"name,omitempty" jsonschema:"New nonblank name; rename to another tag's name is rejected; omitted/null keeps current value"`
	FromFrame        *int    `json:"from_frame,omitempty" jsonschema:"First inclusive frame, 1-based; omitted/null keeps current value"`
	ToFrame          *int    `json:"to_frame,omitempty" jsonschema:"Last inclusive frame, 1-based; omitted/null keeps current value"`
	Direction        *string `json:"direction,omitempty" jsonschema:"forward, reverse, pingpong or pingpong_reverse; omitted/null keeps current value"`
	Repeats          *int    `json:"repeats,omitempty" jsonschema:"0-65535; 0 is native unspecified (infinite UI, once on export); omitted/null keeps current value"`
}

// SetTagPropertiesOutput returns the tag's saved (possibly reordered) identity.
type SetTagPropertiesOutput struct {
	Success  bool      `json:"success"`
	Revision string    `json:"revision"`
	Tag      SpriteTag `json:"tag"`
}

func validateTagProperties(in SetTagPropertiesInput) error {
	ext := strings.ToLower(filepath.Ext(in.SpritePath))
	if (ext != ".ase" && ext != ".aseprite") || in.TagID < 1 || in.TagID > 65535 || !spriteRevisionPattern.MatchString(in.ExpectedRevision) {
		return diagnostics.Errorf("invalid_arguments", "native sprite_path, tag_id and expected_revision are required")
	}
	if in.Name == nil && in.FromFrame == nil && in.ToFrame == nil && in.Direction == nil && in.Repeats == nil {
		return diagnostics.Errorf("invalid_arguments", "at least one tag property is required")
	}
	if in.Name != nil && (strings.TrimSpace(*in.Name) == "" || !utf8.ValidString(*in.Name) || strings.ContainsRune(*in.Name, 0) || len(*in.Name) > 65535) {
		return diagnostics.Errorf("invalid_arguments", "name must be nonblank UTF-8 without NUL and fit 65535 bytes")
	}
	for _, n := range []*int{in.FromFrame, in.ToFrame} {
		if n != nil && (*n < 1 || *n > 65535) {
			return diagnostics.Errorf("invalid_arguments", "tag frames must be between 1 and 65535")
		}
	}
	if in.FromFrame != nil && in.ToFrame != nil && *in.FromFrame > *in.ToFrame {
		return diagnostics.Errorf("invalid_arguments", "tag range must be ordered")
	}
	if in.Repeats != nil && (*in.Repeats < 0 || *in.Repeats > 65535) {
		return diagnostics.Errorf("invalid_arguments", "repeats must be between 0 and 65535")
	}
	if in.Direction != nil {
		switch *in.Direction {
		case "forward", "reverse", "pingpong", "pingpong_reverse":
		default:
			return diagnostics.Errorf("invalid_arguments", "invalid tag direction")
		}
	}
	return nil
}

func registerTagTools(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	mcp.AddTool(server, &mcp.Tool{Name: "get_sprite_tags", Description: "Read all saved tags in native order with names, inclusive 1-based ranges, four playback directions, repeats and RGBA color. Duplicate/empty names are retained. tag_id is an array position valid only with this revision; no save or history."}, maybeWrapConfigured("get_sprite_tags", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, in GetSpriteTagsInput) (*mcp.CallToolResult, *GetSpriteTagsOutput, error) {
		if in.SpritePath == "" {
			return nil, nil, diagnostics.Errorf("invalid_arguments", "sprite_path is required")
		}
		before, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		raw, err := client.ExecuteLua(ctx, gen.GetSpriteTags(), in.SpritePath)
		if err != nil {
			return nil, nil, fmt.Errorf("get sprite tags: %w", err)
		}
		var out GetSpriteTagsOutput
		if err = json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, nil, fmt.Errorf("parse sprite tags: %w", err)
		}
		after, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		if before != after {
			return nil, nil, diagnostics.Errorf("file_changed", "source changed during tag inspection")
		}
		out.Revision = before
		return nil, &out, nil
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "set_tag_properties", Description: "Edit an existing tag using get_sprite_tags tag_id and expected_revision. Supports name, inclusive range, direction and repeats; overlapping tags allowed. Empty/duplicate new names and unchanged requests rejected. Preserves existing empty/duplicate names when not renaming. Returns saved tag_id, which can change when range changes. Refresh tags after any edit."}, maybeWrapConfigured("set_tag_properties", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, in SetTagPropertiesInput) (*mcp.CallToolResult, *SetTagPropertiesOutput, error) {
		if err := validateTagProperties(in); err != nil {
			return nil, nil, err
		}
		// The common wrapper holds the source/history locks and binds a private copy.
		revision, err := aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		if revision != in.ExpectedRevision {
			return nil, nil, diagnostics.Errorf("file_changed", "stale sprite revision; query tags again")
		}
		raw, err := client.ExecuteLua(ctx, gen.SetTagProperties(in.TagID, in.Name, in.FromFrame, in.ToFrame, in.Direction, in.Repeats), in.SpritePath)
		if err != nil {
			return nil, nil, fmt.Errorf("set tag properties: %w", err)
		}
		var out SetTagPropertiesOutput
		if err = json.Unmarshal([]byte(raw), &out); err != nil {
			return nil, nil, fmt.Errorf("parse saved tag: %w", err)
		}
		if !out.Success || out.Tag.TagID < 1 {
			return nil, nil, fmt.Errorf("missing saved tag result")
		}
		out.Revision, err = aseprite.SpriteRevision(ctx, in.SpritePath)
		if err != nil {
			return nil, nil, err
		}
		return nil, &out, nil
	}))
}

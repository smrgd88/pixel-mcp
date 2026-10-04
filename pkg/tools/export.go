package tools

import (
	"context"
	"fmt"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

// ExportSpriteInput defines the input parameters for the export_sprite tool.
type ExportSpriteInput struct {
	Tag              *string `json:"tag,omitempty" jsonschema:"Exact saved native tag name; mutually exclusive with frame range and nonzero frame_number"`
	LayerID          string  `json:"layer_id,omitempty" jsonschema:"Structural layer or group ID from get_sprite_structure; requires expected_revision"`
	ExpectedRevision string  `json:"expected_revision,omitempty" jsonschema:"Saved SHA-256 revision; required with layer_id"`
	FrameStart       *int    `json:"frame_start,omitempty" jsonschema:"First inclusive source frame, 1-based; omitted defaults to 1"`
	FrameEnd         *int    `json:"frame_end,omitempty" jsonschema:"Last inclusive source frame; omitted defaults to last frame"`
	IncludeHidden    bool    `json:"include_hidden,omitempty" jsonschema:"Include hidden target and descendants; requires layer_id; default respects saved visibility"`
	Trim             bool    `json:"trim,omitempty" jsonschema:"Trim transparent frame margins; default false"`
	Overwrite        *bool   `json:"overwrite,omitempty" jsonschema:"Replace existing planned outputs; omitted or null defaults to true; false rejects any existing destination"`

	SpritePath  string `json:"sprite_path" jsonschema:"Path to the Aseprite sprite file"`
	OutputPath  string `json:"output_path" jsonschema:"Output file path; multi-frame PNG/JPG/BMP uses stem_0001.ext, stem_0002.ext, etc."`
	Format      string `json:"format" jsonschema:"Export format: png, gif, jpg, bmp"`
	FrameNumber int    `json:"frame_number" jsonschema:"Specific frame to export (0 = all frames, 1-based)"`
}

// ExportSpriteOutput defines the output for the export_sprite tool.
type ExportSpriteOutput struct {
	ExportedPath string         `json:"exported_path" jsonschema:"Path to the exported file; first file for an image sequence"`
	FileSize     int64          `json:"file_size" jsonschema:"Size of exported_path in bytes; first file only for a sequence"`
	Files        []ExportedFile `json:"files,omitempty" jsonschema:"All generated files in frame order for a multi-file image sequence; omitted for single-file exports"`
}

// ExportedFile identifies one generated sequence frame.
type ExportedFile struct {
	Path        string `json:"path" jsonschema:"Actual output file path"`
	FileSize    int64  `json:"file_size" jsonschema:"Size of this file in bytes"`
	FrameNumber int    `json:"frame_number" jsonschema:"Source frame number (1-based)"`
}

// ExportSpritesheetInput defines the input parameters for the export_spritesheet tool.
type ExportSpritesheetInput struct {
	Tag              *string `json:"tag,omitempty" jsonschema:"Exact saved native tag name; mutually exclusive with frame_start/frame_end"`
	LayerID          string  `json:"layer_id,omitempty" jsonschema:"Structural layer or group ID from get_sprite_structure; requires expected_revision"`
	ExpectedRevision string  `json:"expected_revision,omitempty" jsonschema:"Saved SHA-256 revision; required with layer_id"`
	FrameStart       *int    `json:"frame_start,omitempty" jsonschema:"First inclusive source frame, 1-based; omitted defaults to 1"`
	FrameEnd         *int    `json:"frame_end,omitempty" jsonschema:"Last inclusive source frame; omitted defaults to last frame"`
	IncludeHidden    bool    `json:"include_hidden,omitempty" jsonschema:"Include hidden target and descendants; requires layer_id; default respects saved visibility"`
	Trim             bool    `json:"trim,omitempty" jsonschema:"Trim transparent frame margins; default false"`
	Overwrite        *bool   `json:"overwrite,omitempty" jsonschema:"Replace existing planned outputs; omitted or null defaults to true; false rejects any existing destination"`

	Extrude       bool `json:"extrude,omitempty" jsonschema:"Repeat edge pixels by one pixel outside each sheet frame; default false"`
	BorderPadding *int `json:"border_padding,omitempty" jsonschema:"Sheet outer padding 0-100; overrides padding including explicit zero"`
	ShapePadding  *int `json:"shape_padding,omitempty" jsonschema:"Space between sheet shapes 0-100; overrides padding including explicit zero"`
	InnerPadding  *int `json:"inner_padding,omitempty" jsonschema:"Transparent inset inside each sheet frame 0-100; overrides padding including explicit zero"`

	SpritePath  string `json:"sprite_path" jsonschema:"Path to the Aseprite sprite file"`
	OutputPath  string `json:"output_path" jsonschema:"Output file path for spritesheet"`
	Layout      string `json:"layout" jsonschema:"Spritesheet layout: horizontal, vertical, rows, columns, or packed"`
	Padding     int    `json:"padding" jsonschema:"Default border, shape and inner padding in pixels (0-100); individual options override"`
	IncludeJSON bool   `json:"include_json" jsonschema:"Return metadata_path; the legacy JSON sidecar is always generated and protected"`
}

// ExportSpritesheetOutput defines the output for the export_spritesheet tool.
type ExportSpritesheetOutput struct {
	SpritesheetPath string  `json:"spritesheet_path" jsonschema:"Path to exported spritesheet"`
	MetadataPath    *string `json:"metadata_path,omitempty" jsonschema:"Path to JSON metadata if included"`
	FrameCount      int     `json:"frame_count" jsonschema:"Number of frames in spritesheet"`
}

// ImportImageInput defines the input parameters for the import_image tool.
type ImportImageInput struct {
	SpritePath  string          `json:"sprite_path" jsonschema:"Path to the Aseprite sprite file"`
	ImagePath   string          `json:"image_path" jsonschema:"Path to image file to import"`
	LayerName   string          `json:"layer_name" jsonschema:"Layer name for imported image"`
	FrameNumber int             `json:"frame_number" jsonschema:"Frame number to place image (1-based)"`
	Position    *aseprite.Point `json:"position,omitempty" jsonschema:"Position to place image (optional defaults to 0,0)"`
}

// ImportImageOutput defines the output for the import_image tool.
type ImportImageOutput struct {
	Success bool `json:"success" jsonschema:"Import success status"`
}

// SaveAsInput defines the input parameters for the save_as tool.
type SaveAsInput struct {
	SpritePath string `json:"sprite_path" jsonschema:"Path to the Aseprite sprite file"`
	OutputPath string `json:"output_path" jsonschema:"New .aseprite file path"`
}

// SaveAsOutput defines the output for the save_as tool.
type SaveAsOutput struct {
	Success  bool   `json:"success" jsonschema:"Save success status"`
	FilePath string `json:"file_path" jsonschema:"Path to saved file"`
}

// RegisterExportTools registers all export and import tools with the MCP server.
//
// Registers the following tools:
//   - export_sprite: Export individual frames to PNG, GIF, JPG, or BMP
//   - export_spritesheet: Export all frames as a spritesheet (horizontal/vertical/grid layout)
//   - import_image: Import images as new layers or cels
//   - save_as: Save sprite to a different path
//
// Export tools support multiple output formats and frame selection.
// Import tools can create new layers or merge into existing ones.
func RegisterExportTools(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	// Register export_sprite tool
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "export_sprite",
			Description: "Export sprite to PNG, GIF, JPG, or BMP. All-frame PNG/JPG/BMP exports use numbered files (stem_0001.ext etc.) and return files in frame order. exported_path and file_size identify the first real file. Single-frame exports and animated GIF retain a single output path. The output extension must match format. PNG/JPG/BMP support tag or inclusive frame range, revision-guarded layer/group selection and trim. GIF retains its existing frame_number behavior and rejects these new rendering options.",
		},
		maybeWrapConfigured("export_sprite", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, input ExportSpriteInput) (*mcp.CallToolResult, *ExportSpriteOutput, error) {
			result, err := exportSprite(ctx, client, gen, input)
			return nil, result, err
		}),
	)

	// Register export_spritesheet tool
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "export_spritesheet",
			Description: "Export saved frames as a spritesheet with optional tag/frame range, revision-guarded layer/group selection, trim, extrude and individual padding. Texture and the legacy JSON sidecar are staged and published together with ordinary-error rollback. include_json controls metadata_path in the response; both outputs are always protected.",
		},
		maybeWrapConfigured("export_spritesheet", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, input ExportSpritesheetInput) (*mcp.CallToolResult, *ExportSpritesheetOutput, error) {
			result, err := exportSpritesheet(ctx, client, gen, input)
			return nil, result, err
		}),
	)

	// Register import_image tool
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "import_image",
			Description: "Import an external image file as a layer in the sprite.",
		},
		maybeWrapConfigured("import_image", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, input ImportImageInput) (*mcp.CallToolResult, *ImportImageOutput, error) {
			opLogger := logger.WithContext(ctx)
			opLogger.Debug("import_image tool called", "sprite_path", input.SpritePath, "image_path", input.ImagePath, "layer", input.LayerName)

			// Validate inputs
			if input.SpritePath == "" {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "sprite_path cannot be empty")
			}

			if input.ImagePath == "" {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "image_path cannot be empty")
			}

			if input.LayerName == "" {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "layer_name cannot be empty")
			}

			if input.FrameNumber < 1 {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "frame_number must be >= 1, got %d", input.FrameNumber)
			}

			// Check if image file exists
			if _, err := os.Stat(input.ImagePath); os.IsNotExist(err) {
				return nil, nil, diagnostics.Errorf("not_found", "image file not found: %s", input.ImagePath)
			}

			// Extract position if provided
			var x, y *int
			if input.Position != nil {
				x = &input.Position.X
				y = &input.Position.Y
			}

			// Generate Lua script
			script := gen.ImportImage(input.ImagePath, input.LayerName, input.FrameNumber, x, y)

			// Execute Lua script with the sprite
			output, err := client.ExecuteLua(ctx, script, input.SpritePath)
			if err != nil {
				opLogger.Error("Failed to import image", "error", err)
				return nil, nil, fmt.Errorf("failed to import image: %w", err)
			}

			// Check for success message
			if !strings.Contains(output, "Image imported successfully") {
				opLogger.Warning("Unexpected output from import_image", "output", output)
			}

			opLogger.Information("Image imported successfully", "sprite", input.SpritePath, "image", input.ImagePath, "layer", input.LayerName)

			return nil, &ImportImageOutput{Success: true}, nil
		}),
	)

	// Register save_as tool
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "save_as",
			Description: "Save sprite to a new .aseprite file path.",
		},
		maybeWrapConfigured("save_as", logger, cfg, func(ctx context.Context, req *mcp.CallToolRequest, input SaveAsInput) (*mcp.CallToolResult, *SaveAsOutput, error) {
			opLogger := logger.WithContext(ctx)
			opLogger.Debug("save_as tool called", "sprite_path", input.SpritePath, "output_path", input.OutputPath)

			// Validate inputs
			if input.SpritePath == "" {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "sprite_path cannot be empty")
			}

			if input.OutputPath == "" {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "output_path cannot be empty")
			}

			// Ensure output ends with .aseprite
			if !strings.HasSuffix(input.OutputPath, ".aseprite") && !strings.HasSuffix(input.OutputPath, ".ase") {
				return nil, nil, diagnostics.Errorf("invalid_arguments", "output_path must have .aseprite or .ase extension")
			}

			// Ensure output directory exists
			outputDir := filepath.Dir(input.OutputPath)
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return nil, nil, fmt.Errorf("failed to create output directory: %w", err)
			}

			// Generate Lua script
			script := gen.SaveAs(input.OutputPath)

			// Execute Lua script with the sprite
			output, err := client.ExecuteLua(ctx, script, input.SpritePath)
			if err != nil {
				opLogger.Error("Failed to save sprite", "error", err)
				return nil, nil, fmt.Errorf("failed to save sprite: %w", err)
			}

			// Parse JSON output
			var result SaveAsOutput
			if err := parseJSON(output, &result); err != nil {
				opLogger.Error("Failed to parse save_as output", "error", err, "output", output)
				return nil, nil, fmt.Errorf("failed to parse output: %w", err)
			}

			opLogger.Information("Sprite saved successfully", "sprite", input.SpritePath, "new_path", result.FilePath)

			return nil, &result, nil
		}),
	)
}

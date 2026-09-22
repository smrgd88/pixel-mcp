package tools

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

// QuantizePaletteInput defines the input parameters for the quantize_palette tool.
type QuantizePaletteInput struct {
	SpritePath           string `json:"sprite_path" jsonschema:"Path to source .aseprite file"`
	TargetColors         int    `json:"target_colors" jsonschema:"Maximum palette entries (2-256); indexed output reserves a transparent index and supports at most 255 opaque entries"`
	Algorithm            string `json:"algorithm" jsonschema:"Quantization algorithm: median_cut (default), kmeans, or octree"`
	Dither               bool   `json:"dither" jsonschema:"Apply Floyd-Steinberg dithering during quantization (default: false)"`
	PreserveTransparency *bool  `json:"preserve_transparency,omitempty" jsonschema:"Reserve a palette entry for fully transparent pixels (default: true); transparent pixels remain transparent in either setting"`
	ConvertToIndexed     *bool  `json:"convert_to_indexed,omitempty" jsonschema:"Convert to indexed (default: true); false preserves the input color mode while still remapping pixels"`
}

// QuantizePaletteOutput defines the output for the quantize_palette tool.
type QuantizePaletteOutput struct {
	Warnings        []ToolWarning `json:"warnings,omitempty" jsonschema:"Potentially destructive effects of this completed operation; omitted when none apply"`
	Success         bool          `json:"success" jsonschema:"Whether the operation succeeded"`
	OriginalColors  int           `json:"original_colors" jsonschema:"Number of unique colors in original sprite"`
	QuantizedColors int           `json:"quantized_colors" jsonschema:"Number of colors in quantized palette"`
	ColorMode       string        `json:"color_mode" jsonschema:"Color mode after quantization (indexed, rgb, or grayscale)"`
	Palette         []string      `json:"palette" jsonschema:"Array of hex colors in the quantized palette"`
	AlgorithmUsed   string        `json:"algorithm_used" jsonschema:"Quantization algorithm that was used"`
}

// RegisterQuantizationTools registers the quantize_palette tool with the MCP server.
func RegisterQuantizationTools(server *mcp.Server, client *aseprite.Client, gen *aseprite.LuaGenerator, cfg *config.Config, logger core.Logger) {
	mcp.AddTool(
		server,
		&mcp.Tool{
			Name:        "quantize_palette",
			Description: "Reduce pixels to a quantized palette in single-frame sprites (tilemap layers are unsupported). Supports three algorithms: median_cut (fast, balanced quality), kmeans (highest quality, slower), octree (very fast, good for photos). Can apply Floyd-Steinberg dithering for smoother gradients. Always remaps pixels, even without dithering or indexed conversion. Non-dithered remapping preserves layers; dithering flattens them. Disabling indexed conversion preserves the input color mode. Palette size does not bound colors created by layer blending.",
		},
		maybeWrapWithTiming("quantize_palette", logger, cfg.EnableTiming, cfg.Timeout, func(ctx context.Context, req *mcp.CallToolRequest, input QuantizePaletteInput) (*mcp.CallToolResult, *QuantizePaletteOutput, error) {
			opLogger := logger.WithContext(ctx)
			opLogger.Debug("quantize_palette tool called",
				"sprite", input.SpritePath,
				"target_colors", input.TargetColors,
				"algorithm", input.Algorithm)

			// Set defaults
			if input.Algorithm == "" {
				input.Algorithm = "median_cut"
			}
			if input.PreserveTransparency == nil {
				defaultTrue := true
				input.PreserveTransparency = &defaultTrue
			}
			if input.ConvertToIndexed == nil {
				defaultTrue := true
				input.ConvertToIndexed = &defaultTrue
			}

			// Validate inputs
			if input.TargetColors < 2 || input.TargetColors > 256 {
				return nil, nil, fmt.Errorf("target_colors must be between 2 and 256, got %d", input.TargetColors)
			}

			validAlgorithms := map[string]bool{
				"median_cut": true,
				"kmeans":     true,
				"octree":     true,
			}
			if !validAlgorithms[input.Algorithm] {
				return nil, nil, fmt.Errorf("invalid algorithm: %s (must be median_cut, kmeans, or octree)", input.Algorithm)
			}

			// Check sprite file exists
			if _, err := os.Stat(input.SpritePath); os.IsNotExist(err) {
				return nil, nil, fmt.Errorf("sprite file not found: %s", input.SpritePath)
			}

			// Step 1: Export sprite to temporary PNG for analysis
			tempDir, err := os.MkdirTemp("", "pixel-mcp-quantize-*")
			if err != nil {
				return nil, nil, fmt.Errorf("failed to create temp directory: %w", err)
			}
			defer os.RemoveAll(tempDir)

			tempPNG := filepath.Join(tempDir, "sprite.png")

			// Render one frame and retain the input mode before dithering replaces it.
			exportScript := gen.ExportQuantizationSource(tempPNG)

			sourceOutput, err := client.ExecuteLua(ctx, exportScript, input.SpritePath)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to export sprite to PNG: %w", err)
			}

			var source struct {
				ColorMode string `json:"color_mode"`
			}
			if err := parseJSON(sourceOutput, &source); err != nil {
				return nil, nil, fmt.Errorf("failed to read source color mode: %w", err)
			}

			// Step 2: Load PNG and perform quantization in Go
			imgFile, err := os.Open(tempPNG)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to open exported PNG: %w", err)
			}
			defer imgFile.Close()

			img, err := png.Decode(imgFile)
			if err != nil {
				// Try to decode as any image format
				imgFile.Seek(0, 0)
				img, _, err = image.Decode(imgFile)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to decode image: %w", err)
				}
			}

			// An indexed sprite needs one mask index even when the visible image is
			// opaque. For transparent inputs QuantizePalette already reserves it.
			targetColors := input.TargetColors
			if targetColors == 256 && (*input.ConvertToIndexed || source.ColorMode == "indexed") {
				hasTransparent := false
				if *input.PreserveTransparency {
					for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y && !hasTransparent; y++ {
						for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
							_, _, _, a := img.At(x, y).RGBA()
							if a == 0 {
								hasTransparent = true
								break
							}
						}
					}
				}
				if !hasTransparent {
					targetColors = 255
				}
			}
			// Perform quantization
			palette, originalColors, err := aseprite.QuantizePalette(
				img,
				targetColors,
				input.Algorithm,
				*input.PreserveTransparency,
			)
			if err != nil {
				return nil, nil, fmt.Errorf("quantization failed: %w", err)
			}

			// A grayscale document stores intensity, so report and dither with
			// the same grayscale palette that its cels can represent.
			if source.ColorMode == "grayscale" && !*input.ConvertToIndexed {
				for i, hex := range palette {
					var c aseprite.Color
					if err := c.FromHex(hex); err != nil {
						return nil, nil, err
					}
					if c.A != 0 {
						v := (299*int(c.R) + 587*int(c.G) + 114*int(c.B) + 500) / 1000
						palette[i] = fmt.Sprintf("#%02X%02X%02X", v, v, v)
					}
				}
			}

			opLogger.Information("Quantization completed",
				"original_colors", originalColors,
				"quantized_colors", len(palette),
				"algorithm", input.Algorithm)

			// Step 3: If dithering is requested, remap pixels to quantized palette with dithering
			if input.Dither {
				// Convert hex palette to color.Color slice
				paletteColors := make([]color.Color, len(palette))
				for i, hexColor := range palette {
					var c aseprite.Color
					if err := c.FromHex(hexColor); err != nil {
						return nil, nil, fmt.Errorf("invalid palette color %s: %w", hexColor, err)
					}
					paletteColors[i] = color.NRGBA{R: c.R, G: c.G, B: c.B, A: c.A}
				}

				// Remap image with dithering
				ditheredImg := aseprite.RemapPixelsWithDithering(img, paletteColors, true)

				// Save dithered image to temp file
				ditheredPNG := filepath.Join(tempDir, "dithered.png")
				ditheredFile, err := os.Create(ditheredPNG)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create dithered PNG: %w", err)
				}

				if err := png.Encode(ditheredFile, ditheredImg); err != nil {
					ditheredFile.Close()
					return nil, nil, fmt.Errorf("failed to encode dithered PNG: %w", err)
				}

				if err := ditheredFile.Close(); err != nil {
					return nil, nil, fmt.Errorf("failed to close dithered PNG: %w", err)
				}

				// Replace sprite content with dithered image
				replaceScript := gen.ReplaceWithImage(ditheredPNG)
				_, err = client.ExecuteLua(ctx, replaceScript, input.SpritePath)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to replace sprite with dithered image: %w", err)
				}

				opLogger.Information("Dithering applied successfully",
					"sprite", input.SpritePath)
			}
			// Step 4: Generate and execute Lua script to apply quantized palette
			applyScript := gen.ApplyQuantizedPalette(
				palette,
				originalColors,
				input.Algorithm,
				*input.ConvertToIndexed,
				input.Dither,
				source.ColorMode,
			)

			output, err := client.ExecuteLua(ctx, applyScript, input.SpritePath)
			if err != nil {
				opLogger.Error("Failed to apply quantized palette", "error", err)
				return nil, nil, fmt.Errorf("failed to apply quantized palette: %w", err)
			}

			// Parse JSON output from Lua
			var result QuantizePaletteOutput
			if err := parseJSON(output, &result); err != nil {
				return nil, nil, fmt.Errorf("failed to parse quantization output: %w", err)
			}

			opLogger.Information("Palette quantized and applied successfully",
				"sprite", input.SpritePath,
				"original_colors", result.OriginalColors,
				"quantized_colors", result.QuantizedColors,
				"color_mode", result.ColorMode,
				"algorithm", result.AlgorithmUsed)

			result.Warnings = quantizationWarnings(*input.ConvertToIndexed, input.Dither)
			return nil, &result, nil
		}),
	)
}

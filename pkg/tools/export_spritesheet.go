package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func exportSpritesheet(ctx context.Context, c *aseprite.Client, g *aseprite.LuaGenerator, in ExportSpritesheetInput) (*ExportSpritesheetOutput, error) {
	if in.SpritePath == "" || in.OutputPath == "" {
		return nil, diagnostics.Errorf("invalid_arguments", "sprite_path and output_path are required")
	}
	layout := in.Layout
	if layout == "" {
		layout = "horizontal"
	}
	switch layout {
	case "horizontal", "vertical", "rows", "columns", "packed":
	default:
		return nil, diagnostics.Errorf("invalid_arguments", "invalid layout")
	}
	ext := strings.ToLower(filepath.Ext(in.OutputPath))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp":
	default:
		return nil, diagnostics.Errorf("invalid_arguments", "spritesheet requires PNG, JPG, GIF or BMP texture extension")
	}
	if in.Padding < 0 || in.Padding > 100 {
		return nil, diagnostics.Errorf("invalid_arguments", "padding must be between 0 and 100")
	}
	pads := []int{in.Padding, in.Padding, in.Padding}
	for i, v := range []*int{in.BorderPadding, in.ShapePadding, in.InnerPadding} {
		if v != nil {
			if *v < 0 || *v > 100 {
				return nil, diagnostics.Errorf("invalid_arguments", "individual padding must be between 0 and 100")
			}
			pads[i] = *v
		}
	}
	selection, err := validateExportSelection(in.Tag, in.LayerID, in.ExpectedRevision, in.FrameStart, in.FrameEnd, in.IncludeHidden, 0)
	if err != nil {
		return nil, err
	}
	// Preserve the legacy sidecar even when include_json omits metadata_path.
	paths := []string{in.OutputPath, spritesheetDataPath(in.OutputPath)}
	var plan exportPlan
	err = aseprite.WithFileLocks(ctx, append([]string{in.SpritePath}, paths...), func(ctx context.Context) error {
		if err := validateSheetOutputs(in.SpritePath, paths); err != nil {
			return err
		}
		return aseprite.WithSpriteAccess(ctx, in.SpritePath, false, func(ctx context.Context) error {
			var err error
			plan, err = planExport(ctx, c, g, in.SpritePath, in.ExpectedRevision, selection)
			if err != nil {
				return err
			}
			if err = validateExportDimensions(plan, layout, pads[0], pads[1], pads[2], in.Extrude); err != nil {
				return err
			}
			if err = checkExportOverwrite(paths, in.Overwrite); err != nil {
				return err
			}
			return withExportOutputFiles(ctx, paths, in.Overwrite, func(staged []string) error {
				if strings.ToLower(filepath.Ext(staged[0])) != ext || strings.ToLower(filepath.Ext(staged[1])) != ".json" {
					return diagnostics.Errorf("invalid_arguments", "canonical output extensions must match the planned texture and JSON")
				}
				raw, err := c.ExecuteLua(ctx, g.ExportSelectedSheet(staged[0], staged[1], layout, pads[0], pads[1], pads[2], in.Trim, in.Extrude, selection), in.SpritePath)
				if err != nil {
					return fmt.Errorf("failed to export spritesheet: %w", err)
				}
				var size struct {
					Width  int `json:"width"`
					Height int `json:"height"`
				}
				if err = parseJSON(raw, &size); err != nil {
					return err
				}
				if size.Width < 1 || size.Height < 1 || size.Width > 32767 || size.Height > 32767 || int64(size.Width)*int64(size.Height) > 64*1024*1024 {
					return fmt.Errorf("invalid spritesheet dimensions")
				}
				// Native reopening covers BMP as well. Independently decode other formats.
				if ext != ".bmp" {
					f, err := os.Open(staged[0])
					if err != nil {
						return err
					}
					cfg, _, err := image.DecodeConfig(f)
					f.Close()
					if err != nil {
						return err
					}
					if cfg.Width != size.Width || cfg.Height != size.Height {
						return fmt.Errorf("texture dimensions mismatch")
					}
				}
				if err = validateSheetMetadata(staged[1], filepath.Base(paths[0]), plan, size.Width, size.Height); err != nil {
					return err
				}
				return verifyExportRevision(ctx, in.SpritePath, plan.Revision)
			})
		})
	})
	if err != nil {
		return nil, err
	}
	result := &ExportSpritesheetOutput{SpritesheetPath: in.OutputPath, FrameCount: plan.Last - plan.First + 1}
	if in.IncludeJSON {
		result.MetadataPath = &paths[1]
	}
	return result, nil
}

// validateSheetMetadata preserves native fields while checking the ordered frame
// set against the plan. Metadata filenames must refer to published artifacts,
// never private staging paths. Original source numbers are additive metadata.
func validateSheetMetadata(path, texture string, p exportPlan, width, height int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if err = json.Unmarshal(escapeNativeJSONControls(data), &root); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(root["frames"]))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return fmt.Errorf("missing spritesheet frame object")
	}
	frames := map[string]json.RawMessage{}
	i := 0
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return err
		}
		name, ok := key.(string)
		if !ok {
			return fmt.Errorf("invalid frame key")
		}
		if _, exists := frames[name]; exists {
			return fmt.Errorf("duplicate frame key")
		}
		var row map[string]json.RawMessage
		if err = dec.Decode(&row); err != nil {
			return err
		}
		var bounds struct{ X, Y, W, H int }
		if err = json.Unmarshal(row["frame"], &bounds); err != nil {
			return err
		}
		if bounds.X < 0 || bounds.Y < 0 || bounds.W < 1 || bounds.H < 1 || bounds.W > width || bounds.H > height || bounds.X > width-bounds.W || bounds.Y > height-bounds.H {
			return fmt.Errorf("frame outside spritesheet texture")
		}
		var duration int
		if err = json.Unmarshal(row["duration"], &duration); err != nil {
			return err
		}
		if i >= len(p.Durations) || duration != p.Durations[i] {
			return fmt.Errorf("spritesheet frame duration/count mismatch")
		}
		var source struct{ W, H int }
		if err = json.Unmarshal(row["sourceSize"], &source); err != nil {
			return err
		}
		if source.W != p.Width || source.H != p.Height {
			return fmt.Errorf("spritesheet source size mismatch")
		}
		var offset struct{ X, Y, W, H int }
		if err = json.Unmarshal(row["spriteSourceSize"], &offset); err != nil {
			return err
		}
		if offset.X < 0 || offset.Y < 0 || offset.W < 0 || offset.H < 0 || offset.W > p.Width || offset.H > p.Height || offset.X > p.Width-offset.W || offset.Y > p.Height-offset.H {
			return fmt.Errorf("invalid spritesheet trim bounds")
		}
		row["source_frame_number"], _ = json.Marshal(p.First + i)
		frames[name], err = json.Marshal(row)
		if err != nil {
			return err
		}
		i++
	}
	if i != p.Last-p.First+1 {
		return fmt.Errorf("spritesheet frame count mismatch")
	}
	var meta map[string]json.RawMessage
	if err = json.Unmarshal(root["meta"], &meta); err != nil {
		return err
	}
	var size struct{ W, H int }
	if err = json.Unmarshal(meta["size"], &size); err != nil {
		return err
	}
	if size.W != width || size.H != height {
		return fmt.Errorf("spritesheet metadata size mismatch")
	}
	meta["image"], _ = json.Marshal(texture)
	// Native tag indices describe the original document. Clip and translate them
	// to the selected output timeline without changing direction/color/custom data.
	if raw, ok := meta["frameTags"]; ok {
		var tags []map[string]json.RawMessage
		if err = json.Unmarshal(raw, &tags); err != nil {
			return err
		}
		selected := make([]map[string]json.RawMessage, 0, len(tags))
		for _, tag := range tags {
			var first, last int
			if err = json.Unmarshal(tag["from"], &first); err != nil {
				return err
			}
			if err = json.Unmarshal(tag["to"], &last); err != nil {
				return err
			}
			first = max(first, p.First-1)
			last = min(last, p.Last-1)
			if first > last {
				continue
			}
			tag["from"], _ = json.Marshal(first - (p.First - 1))
			tag["to"], _ = json.Marshal(last - (p.First - 1))
			selected = append(selected, tag)
		}
		meta["frameTags"], err = json.Marshal(selected)
		if err != nil {
			return err
		}
	}
	root["meta"], err = json.Marshal(meta)
	if err != nil {
		return err
	}
	root["frames"], err = json.Marshal(frames)
	if err != nil {
		return err
	}
	data, err = json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

// Aseprite 1.3.x's sheet serializer escapes quotes/backslashes but leaves control
// bytes in metadata strings raw. Normalize only those bytes; malformed syntax
// still fails strict JSON decoding, and user names/data retain their values.
func escapeNativeJSONControls(data []byte) []byte {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(data))
	inString, escaped := false, false
	for _, b := range data {
		if inString && b < 0x20 {
			out = append(out, '\\', 'u', '0', '0', hex[b>>4], hex[b&15])
		} else {
			out = append(out, b)
		}
		if escaped {
			escaped = false
			continue
		}
		if inString && b == '\\' {
			escaped = true
			continue
		}
		if b == '"' {
			inString = !inString
		}
	}
	return out
}

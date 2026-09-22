package tools

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// The input types use these common Go fields for their documented JSON paths.
// Reflection is limited to string fields in a copied input struct; input schemas
// and caller-owned inputs are never modified.
func inputPath(input any, field string) string {
	v := reflect.ValueOf(input)
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName(field)
	if f.IsValid() && f.Kind() == reflect.String {
		return f.String()
	}
	return ""
}
func withInputPath[I any](input I, field, path string) I {
	v := reflect.New(reflect.TypeOf(input)).Elem()
	v.Set(reflect.ValueOf(input))
	v.FieldByName(field).SetString(path)
	return v.Interface().(I)
}

func wrapWithFileProtection[I, O any](tool string, timeout time.Duration, handler func(context.Context, *mcp.CallToolRequest, I) (*mcp.CallToolResult, O, error)) func(context.Context, *mcp.CallToolRequest, I) (*mcp.CallToolResult, O, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, input I) (*mcp.CallToolResult, O, error) {
		source := inputPath(input, "SpritePath")
		if source == "" {
			source = inputPath(input, "SourcePath")
		}
		if source == "" {
			source = inputPath(input, "ReferencePath")
		}
		if source == "" {
			return handler(ctx, req, input)
		} // create_canvas uses a unique generated path.
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		var result *mcp.CallToolResult
		var output O
		call := func(ctx context.Context, in I) error {
			var err error
			result, output, err = handler(ctx, req, in)
			if err == nil && result != nil && result.IsError {
				return fmt.Errorf("tool returned an error result")
			}
			return err
		}
		target := inputPath(input, "OutputPath")
		paths := []string{source, target, inputPath(input, "ImagePath")}
		err := aseprite.WithFileLocks(ctx, paths, func(ctx context.Context) error {
			// Keep argument validation and missing-file errors in the existing handlers.
			if _, e := os.Stat(source); e != nil {
				return call(ctx, input)
			}
			readOnly := false
			switch tool {
			case "get_sprite_info", "get_pixels", "get_palette", "analyze_palette_harmonies", "analyze_reference", "export_sprite", "export_spritesheet", "save_as", "downsample_image":
				readOnly = true
			}
			if aa, ok := any(input).(SuggestAntialiasingInput); ok && !aa.AutoApply {
				readOnly = true
			}
			if tool == "save_as" || tool == "downsample_image" || tool == "export_sprite" {
				return aseprite.WithSpriteAccess(ctx, source, false, func(ctx context.Context) error {
					if target == "" {
						return call(ctx, input)
					}
					err := aseprite.WithOutputFile(ctx, target, func(staged string) error { return call(ctx, withInputPath(input, "OutputPath", staged)) })
					if err == nil {
						restoreOutputPath(output, target)
					}
					return err
				})
			}
			if (tool == "export_sprite" || tool == "export_spritesheet") && target != "" {
				src, e := aseprite.CanonicalPath(source)
				if e != nil {
					return e
				}
				dst, e := aseprite.CanonicalPath(target)
				if e != nil {
					return e
				}
				si, e := os.Stat(src)
				if e != nil {
					return e
				}
				di, e := os.Stat(dst)
				if src == dst || (e == nil && os.SameFile(si, di)) {
					return fmt.Errorf("export output must not overwrite the source sprite")
				}
			}
			return aseprite.WithSpriteAccess(ctx, source, !readOnly, func(ctx context.Context) error { return call(ctx, input) })
		})
		if err != nil {
			var zero O
			return nil, zero, err
		}
		return result, output, nil
	}
}

func restoreOutputPath(output any, path string) {
	switch out := output.(type) {
	case *SaveAsOutput:
		out.FilePath = path
	case *ExportSpriteOutput:
		out.ExportedPath = path
	case *DownsampleImageOutput:
		out.OutputPath = path
	}
}

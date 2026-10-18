package tools

import (
	"context"
	"fmt"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"os"
	"path/filepath"
	"strings"

	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func exportSprite(ctx context.Context, client *aseprite.Client, gen *aseprite.LuaGenerator, input ExportSpriteInput) (*ExportSpriteOutput, error) {
	if input.SpritePath == "" {
		return nil, diagnostics.Errorf("invalid_arguments", "sprite_path cannot be empty")
	}
	if input.OutputPath == "" {
		return nil, diagnostics.Errorf("invalid_arguments", "output_path cannot be empty")
	}
	format := strings.ToLower(input.Format)
	if format != "png" && format != "gif" && format != "jpg" && format != "bmp" {
		return nil, diagnostics.Errorf("invalid_arguments", "invalid format: %s (valid: png, gif, jpg, bmp)", input.Format)
	}
	ext := strings.ToLower(filepath.Ext(input.OutputPath))
	if ext != "."+format && !(format == "jpg" && ext == ".jpeg") {
		return nil, diagnostics.Errorf("invalid_arguments", "output_path extension must match format %s", format)
	}
	if input.FrameNumber < 0 {
		return nil, diagnostics.Errorf("invalid_arguments", "frame_number must be non-negative")
	}
	selection, err := validateExportSelection(input.Tag, input.LayerID, input.ExpectedRevision, input.FrameStart, input.FrameEnd, input.IncludeHidden, input.FrameNumber)
	if err != nil {
		return nil, err
	}
	if format == "gif" && (selection.Selected || input.Trim) {
		return nil, diagnostics.Errorf("invalid_arguments", "GIF does not support new selection or trim options")
	}
	// Plan under the source lock, release it, then acquire the entire sorted set.
	planningSelection := selection
	// Legacy frame_number range errors are Go invalid_arguments, not lua_error.
	if input.FrameNumber > 0 {
		planningSelection.First = 0
		planningSelection.Last = 0
	}
	plan, err := planExport(ctx, client, gen, input.SpritePath, input.ExpectedRevision, planningSelection)
	if err != nil {
		return nil, err
	}
	if input.FrameNumber > plan.Count {
		return nil, diagnostics.Errorf("invalid_arguments", "frame_number outside sprite frame range")
	}
	if err = validateExportDimensions(plan, "", 0, 0, 0, false); err != nil {
		return nil, err
	}
	paths, frames := exportFramePaths(input.OutputPath, format, input.FrameNumber, plan.Count)
	if selection.Selected && (input.Tag != nil || input.FrameStart != nil || input.FrameEnd != nil) {
		paths, frames = exportRangePaths(input.OutputPath, plan.First, plan.Last)
	}
	locks := append([]string{input.SpritePath, input.OutputPath}, paths...)
	sizes := make([]int64, len(paths))
	err = aseprite.WithFileLocks(ctx, locks, func(ctx context.Context) error {
		if err := validateExportSourceAliases(input.SpritePath, append([]string{input.OutputPath}, paths...)); err != nil {
			return err
		}
		return aseprite.WithSpriteAccess(ctx, input.SpritePath, false, func(ctx context.Context) error {
			if err := verifyExportRevision(ctx, input.SpritePath, plan.Revision); err != nil {
				return err
			}
			if err := checkExportOverwrite(paths, input.Overwrite); err != nil {
				return err
			}
			return withExportOutputFiles(ctx, paths, input.Overwrite, func(staged []string) error {
				for _, path := range staged {
					ext := strings.ToLower(filepath.Ext(path))
					if ext != "."+format && !(format == "jpg" && ext == ".jpeg") {
						return diagnostics.Errorf("invalid_arguments", "canonical output extension must match format")
					}
				}
				script := gen.ExportSelectedFiles(staged, frames, plan.Count, selection, input.Trim)
				if _, err := client.ExecuteLua(ctx, script, input.SpritePath); err != nil {
					return fmt.Errorf("failed to export sprite: %w", err)
				}
				for i, path := range staged {
					st, err := os.Stat(path)
					if err != nil {
						return err
					}
					sizes[i] = st.Size()
				}
				return verifyExportRevision(ctx, input.SpritePath, plan.Revision)
			})
		})
	})
	if err != nil {
		return nil, err
	}
	result := &ExportSpriteOutput{ExportedPath: paths[0], FileSize: sizes[0]}
	if len(paths) > 1 {
		for i, path := range paths {
			result.Files = append(result.Files, ExportedFile{Path: path, FileSize: sizes[i], FrameNumber: frames[i]})
		}
	}
	return result, nil
}

func exportFramePaths(path, format string, frame, count int) ([]string, []int) {
	if frame > 0 {
		return []string{path}, []int{frame}
	}
	if format == "gif" {
		return []string{path}, []int{0}
	}
	if count == 1 {
		return []string{path}, []int{1}
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	paths := make([]string, count)
	frames := make([]int, count)
	for i := range paths {
		frames[i] = i + 1
		paths[i] = fmt.Sprintf("%s_%04d%s", stem, i+1, ext)
	}
	return paths, frames
}

func validateExportSourceAliases(source string, paths []string) error {
	src, err := aseprite.CanonicalPath(source)
	if err != nil {
		return err
	}
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	for _, path := range paths {
		dest, err := aseprite.CanonicalPath(path)
		if err != nil {
			return err
		}
		other, err := os.Stat(dest)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if dest == src || (other != nil && os.SameFile(info, other)) {
			return diagnostics.Errorf("invalid_arguments", "export output must not alias source")
		}
	}
	return nil
}

// A single selected frame retains the requested path. A sequence uses original
// source frame numbers, so range 3..4 is stem_0003.ext and stem_0004.ext.
func exportRangePaths(path string, first, last int) ([]string, []int) {
	if first == last {
		return []string{path}, []int{first}
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	paths := make([]string, last-first+1)
	frames := make([]int, len(paths))
	for i := range paths {
		frames[i] = first + i
		paths[i] = fmt.Sprintf("%s_%04d%s", stem, frames[i], ext)
	}
	return paths, frames
}

package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func exportSprite(ctx context.Context, client *aseprite.Client, gen *aseprite.LuaGenerator, input ExportSpriteInput) (*ExportSpriteOutput, error) {
	if input.SpritePath == "" {
		return nil, fmt.Errorf("sprite_path cannot be empty")
	}
	if input.OutputPath == "" {
		return nil, fmt.Errorf("output_path cannot be empty")
	}
	format := strings.ToLower(input.Format)
	if format != "png" && format != "gif" && format != "jpg" && format != "bmp" {
		return nil, fmt.Errorf("invalid format: %s (valid: png, gif, jpg, bmp)", input.Format)
	}
	ext := strings.ToLower(filepath.Ext(input.OutputPath))
	if ext != "."+format && !(format == "jpg" && ext == ".jpeg") {
		return nil, fmt.Errorf("output_path extension must match format %s", format)
	}
	if input.FrameNumber < 0 {
		return nil, fmt.Errorf("frame_number must be non-negative")
	}
	// Planning takes only the source lock. Release it before acquiring the full
	// sorted set, avoiding lock-order inversion with overlapping exports/edits.
	var count struct {
		Frames int `json:"frames"`
	}
	err := aseprite.WithSpriteAccess(ctx, input.SpritePath, false, func(ctx context.Context) error {
		out, err := client.ExecuteLua(ctx, `local s=app.activeSprite;if not s then error("No active sprite") end;print(json.encode({frames=#s.frames}))`, input.SpritePath)
		if err != nil {
			return err
		}
		return parseJSON(out, &count)
	})
	if err != nil {
		return nil, err
	}
	if count.Frames < 1 || input.FrameNumber > count.Frames {
		return nil, fmt.Errorf("frame_number outside sprite frame range")
	}
	paths, frames := exportFramePaths(input.OutputPath, format, input.FrameNumber, count.Frames)
	locks := append([]string{input.SpritePath, input.OutputPath}, paths...)
	sizes := make([]int64, len(paths))
	err = aseprite.WithFileLocks(ctx, locks, func(ctx context.Context) error {
		if err := validateExportSourceAliases(input.SpritePath, append([]string{input.OutputPath}, paths...)); err != nil {
			return err
		}
		return aseprite.WithSpriteAccess(ctx, input.SpritePath, false, func(ctx context.Context) error {
			return aseprite.WithOutputFiles(ctx, paths, func(staged []string) error {
				script := gen.ExportSpriteFiles(staged, frames, count.Frames)
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
				return nil
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
			return fmt.Errorf("export output must not alias source")
		}
	}
	return nil
}

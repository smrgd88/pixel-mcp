package tools

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

// loadReferenceImage preserves PNG/JPEG/GIF decoding behavior and uses Aseprite
// only for recognized BMP/native signatures. The MCP wrapper holds the reference
// read lock for the entire analysis; ExecuteLua reuses that read-only binding.
func loadReferenceImage(ctx context.Context, client *aseprite.Client, gen *aseprite.LuaGenerator, path, tempRoot string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open reference image: %w", err)
	}
	img, _, decodeErr := image.Decode(file)
	var header [6]byte
	n, _ := file.ReadAt(header[:], 0)
	file.Close()
	if decodeErr == nil {
		return img, nil
	}
	// BMP starts with BM. The little-endian Aseprite header has magic 0xA5E0
	// at bytes 4-5, after its DWORD file size.
	// https://github.com/aseprite/aseprite/blob/main/docs/ase-file-specs.md
	bmp := n >= 2 && header[0] == 'B' && header[1] == 'M'
	native := n >= 6 && header[4] == 0xe0 && header[5] == 0xa5
	if !bmp && !native {
		return nil, fmt.Errorf("failed to decode reference image: %w", decodeErr)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tempRoot != "" {
		if err := os.MkdirAll(tempRoot, 0700); err != nil {
			return nil, fmt.Errorf("failed to prepare reference temp directory: %w", err)
		}
	}
	dir, err := os.MkdirTemp(tempRoot, "pixel-mcp-reference-")
	if err != nil {
		return nil, fmt.Errorf("failed to create reference temp directory: %w", err)
	}
	defer os.RemoveAll(dir)
	rendered := filepath.Join(dir, "frame.png")
	if _, err := client.ExecuteLua(ctx, gen.RenderReferenceImage(rendered), path); err != nil {
		return nil, fmt.Errorf("failed to render reference image: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	converted, err := os.Open(rendered)
	if err != nil {
		return nil, fmt.Errorf("failed to open rendered reference: %w", err)
	}
	defer converted.Close()
	img, err = png.Decode(converted)
	if err != nil {
		return nil, fmt.Errorf("failed to decode rendered reference: %w", err)
	}
	return img, nil
}

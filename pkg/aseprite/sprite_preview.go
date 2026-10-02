package aseprite

import (
	"context"
	"fmt"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"io"
	"os"
	"path/filepath"
)

// WithSpritePreview binds Client.ExecuteLua's logical source path to a private,
// writable copy for the complete callback. It never publishes that copy. Callers
// must perform sprite writes through the bound Client, not directly to path.
func WithSpritePreview(ctx context.Context, path string, fn func(context.Context, string) error) error {
	return WithFileLocks(ctx, []string{path}, func(ctx context.Context) error {
		original, err := CanonicalPath(path)
		if err != nil {
			return err
		}
		scope := scopeOf(ctx)
		key := pathKey(original)
		if !scope.locked[key] {
			return diagnostics.Errorf("file_changed", "file_changed: preview source target changed")
		}
		if _, bound := scope.sprites[key]; bound {
			return fmt.Errorf("sprite_preview_scope: source already bound to an operation")
		}
		before, err := snapshot(original)
		if err != nil {
			return err
		}
		validate := func() error {
			currentPath, err := CanonicalPath(path)
			if err != nil {
				return err
			}
			if currentPath != original {
				return diagnostics.Errorf("file_changed", "file_changed: preview source path changed")
			}
			current, err := snapshot(original)
			if err != nil || !os.SameFile(before.info, current.info) || before.hash != current.hash || before.info.Mode() != current.info.Mode() {
				return diagnostics.Errorf("file_changed", "file_changed: source changed during preview")
			}
			return ctx.Err()
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		dir, err := os.MkdirTemp("", "pixel-mcp-preview-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		copyPath := filepath.Join(dir, filepath.Base(original))
		src, _, err := openRegularFile(original)
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(copyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dst, src)
		src.Close()
		closeErr := dst.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		copied, err := snapshot(copyPath)
		if err != nil {
			return err
		}
		if copied.hash != before.hash {
			return diagnostics.Errorf("file_changed", "file_changed: source changed while copying for preview")
		}
		if err := validate(); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		child := &fileScope{locked: scope.locked, sprites: make(map[string]string, len(scope.sprites)+1)}
		for k, v := range scope.sprites {
			child.sprites[k] = v
		}
		child.sprites[key] = copyPath
		if err := fn(context.WithValue(ctx, fileContextKey{}, child), copyPath); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return validate()
	})
}

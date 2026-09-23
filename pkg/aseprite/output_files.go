package aseprite

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WithOutputFiles stages a known output set before publishing any file. All
// paths are locked together. A normal commit error/cancellation rolls back files
// already replaced; a rollback failure retains backups and reports their paths.
// Individual replacements are atomic, but the set is not crash-atomic.
func WithOutputFiles(ctx context.Context, paths []string, fn func([]string) error) error {
	return withOutputFilesReplace(ctx, paths, fn, replaceFile)
}

type stagedOutput struct {
	path, original, dir, staged, backup string
	before, after                       fileSnapshot
	existed, committed                  bool
}

func withOutputFilesReplace(ctx context.Context, paths []string, fn func([]string) error, replace func(string, string) error) error {
	if len(paths) == 0 {
		return fmt.Errorf("output set cannot be empty")
	}
	return WithFileLocks(ctx, paths, func(ctx context.Context) error {
		items := make([]*stagedOutput, 0, len(paths))
		retain := false
		defer func() {
			if !retain {
				for _, item := range items {
					if item.dir != "" {
						os.RemoveAll(item.dir)
					}
				}
			}
		}()
		keys := map[string]bool{}
		staged := make([]string, 0, len(paths))
		for _, path := range paths {
			original, err := CanonicalPath(path)
			if err != nil {
				return err
			}
			key := pathKey(original)
			if keys[key] {
				return fmt.Errorf("duplicate output destination: %s", path)
			}
			keys[key] = true
			if _, bound := scopeOf(ctx).sprites[key]; bound {
				return fmt.Errorf("output must not alias a bound source: %s", path)
			}
			if !scopeOf(ctx).locked[key] {
				return fmt.Errorf("file_changed: output target changed before staging")
			}
			item := &stagedOutput{path: path, original: original}
			items = append(items, item)
			if err := item.prepare(ctx); err != nil {
				return err
			}
			staged = append(staged, item.staged)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(staged); err != nil {
			return err
		}
		// Complete validation and syncing for every output before the first publish.
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			st, err := os.Lstat(item.staged)
			if err != nil {
				return err
			}
			if !st.Mode().IsRegular() || st.Size() == 0 {
				return fmt.Errorf("invalid staged output: %s", item.path)
			}
			mode := os.FileMode(0600)
			if item.existed {
				mode = item.before.info.Mode().Perm()
			}
			if err := os.Chmod(item.staged, mode); err != nil {
				return err
			}
			if err := syncOutput(item.staged); err != nil {
				return err
			}
			item.after, err = snapshot(item.staged)
			if err != nil {
				return err
			}
			if err := item.validateOriginal(); err != nil {
				return err
			}
		}
		rollback := func(cause error) error {
			var recovery []error
			for i := len(items) - 1; i >= 0; i-- {
				item := items[i]
				if !item.committed {
					continue
				}
				// Do not overwrite a non-cooperating writer's changes during recovery.
				currentPath, err := CanonicalPath(item.path)
				if err == nil && currentPath != item.original {
					err = fmt.Errorf("output path changed")
				}
				if err == nil {
					current, e := snapshot(item.original)
					err = e
					if err == nil && (!os.SameFile(item.after.info, current.info) || item.after.hash != current.hash || item.after.info.Mode() != current.info.Mode()) {
						err = fmt.Errorf("published output changed")
					}
				}
				if err == nil {
					if item.existed {
						err = replace(item.backup, item.original)
					} else {
						err = os.Remove(item.original)
					}
				}
				if err != nil {
					recovery = append(recovery, fmt.Errorf("file_rollback_failed: %s: %w; recovery directory: %s", item.path, err, item.dir))
				}
			}
			if len(recovery) > 0 {
				retain = true
				return errors.Join(append([]error{cause}, recovery...)...)
			}
			return cause
		}
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return rollback(err)
			}
			if err := item.validateOriginal(); err != nil {
				return rollback(err)
			}
			if item.existed && item.before.hash == item.after.hash {
				continue
			}
			if err := replace(item.staged, item.original); err != nil {
				return rollback(fmt.Errorf("file_commit_failed: %w", err))
			}
			item.committed = true
		}
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		return nil
	})
}

func (item *stagedOutput) prepare(ctx context.Context) error {
	var err error
	item.before, err = snapshot(item.original)
	item.existed = err == nil
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if item.existed {
		if item.before.info.Mode().Perm()&0222 == 0 {
			return fmt.Errorf("file is read-only: %s", item.path)
		}
		if err := checkSingleLink(item.original, item.before.info); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(item.original), 0755); err != nil {
		return err
	}
	item.dir, err = os.MkdirTemp(filepath.Dir(item.original), ".pixel-mcp-stage-")
	if err != nil {
		return err
	}
	item.staged = filepath.Join(item.dir, filepath.Base(item.original))
	if item.existed {
		item.backup = filepath.Join(item.dir, ".original-backup")
		// A target with this basename must not share its staging path with backup.
		if item.backup == item.staged {
			item.backup = filepath.Join(item.dir, ".original-backup-2")
		}
		src, _, err := openRegularFile(item.original)
		if err != nil {
			return err
		}
		dst, err := os.OpenFile(item.backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
		if err := os.Chmod(item.backup, item.before.info.Mode().Perm()); err != nil {
			return err
		}
		if err := syncOutput(item.backup); err != nil {
			return err
		}
		backup, err := snapshot(item.backup)
		if err != nil {
			return err
		}
		if backup.hash != item.before.hash {
			return fmt.Errorf("file_changed: output changed while backing up")
		}
	}
	return nil
}

func (item *stagedOutput) validateOriginal() error {
	path, err := CanonicalPath(item.path)
	if err != nil {
		return err
	}
	if path != item.original {
		return fmt.Errorf("file_changed: output path changed")
	}
	current, err := snapshot(item.original)
	if !item.existed {
		if !os.IsNotExist(err) {
			return fmt.Errorf("file_changed: output appeared during export")
		}
		return nil
	}
	if err != nil || !os.SameFile(item.before.info, current.info) || item.before.hash != current.hash || item.before.info.Mode() != current.info.Mode() {
		return fmt.Errorf("file_changed: output changed during export")
	}
	return checkSingleLink(item.original, current.info)
}

func syncOutput(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

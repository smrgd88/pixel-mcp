package aseprite

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type fileContextKey struct{}
type fileScope struct {
	locked  map[string]bool
	sprites map[string]string
}

func scopeOf(ctx context.Context) *fileScope {
	s, _ := ctx.Value(fileContextKey{}).(*fileScope)
	return s
}
func pathKey(path string) string {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.ToLower(path)
	}
	return path
}

// CanonicalPath resolves existing symlinks, including the nearest existing parent
// of a new output. It never creates a directory or file.
func CanonicalPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("file path cannot be empty")
	}
	// Abs/Clean would collapse link/.. before symlink resolution and can select
	// a different file from the path that the OS opens.
	abs := path
	if !filepath.IsAbs(abs) {
		if filepath.VolumeName(abs) != "" {
			return "", fmt.Errorf("drive-relative paths are unsupported: %s", path)
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		if runtime.GOOS == "windows" && os.IsPathSeparator(abs[0]) {
			abs = filepath.VolumeName(cwd) + abs
		} else {
			abs = cwd + string(filepath.Separator) + abs
		}
	}
	for len(abs) > len(filepath.VolumeName(abs))+1 && os.IsPathSeparator(abs[len(abs)-1]) {
		abs = abs[:len(abs)-1]
	}
	p, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return p, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	if _, e := os.Lstat(abs); e == nil {
		return "", fmt.Errorf("dangling symlink: %s", path)
	}
	parent, base := filepath.Split(abs)
	if base == "." || base == ".." {
		return "", fmt.Errorf("unresolved parent traversal: %s", path)
	}
	if parent == abs || parent == "" {
		return "", err
	}
	resolved, err := CanonicalPath(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, base), nil
}

// WithFileLocks serializes cooperating callers (including readers) for the entire
// operation, across clients and processes of the same user. Locks are acquired in
// sorted order. Persistent lock files must not be deleted while servers run.
func WithFileLocks(ctx context.Context, paths []string, fn func(context.Context) error) error {
	keys := map[string]bool{}
	for _, p := range paths {
		if p == "" {
			continue
		}
		c, e := CanonicalPath(p)
		if e != nil {
			return e
		}
		keys[pathKey(c)] = true
	}
	scope := scopeOf(ctx)
	if scope != nil {
		for k := range keys {
			if !scope.locked[k] {
				return fmt.Errorf("file_lock_scope: nested operation must reserve all paths up front")
			}
		}
		return fn(ctx)
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	var locks []*os.File
	defer func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].Close()
		}
	}()
	for _, k := range ordered {
		f, e := acquireFileLock(ctx, k)
		if e != nil {
			return e
		}
		locks = append(locks, f)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(context.WithValue(ctx, fileContextKey{}, &fileScope{locked: keys, sprites: map[string]string{}}))
}

func acquireFileLock(ctx context.Context, key string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(cache, "pixel-mcp", "file-locks")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid file lock directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("file lock directory must be private: %s", dir)
	}
	path := filepath.Join(dir, fmt.Sprintf("%x.lock", sha256.Sum256([]byte(key))))
	if info, e := os.Lstat(path); e == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid lock file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err = ctx.Err(); err != nil {
			f.Close()
			return nil, fmt.Errorf("file_lock_wait: %w", err)
		}
		acquired, e := tryFileLock(f)
		if e != nil {
			f.Close()
			return nil, e
		}
		if acquired {
			return f, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			f.Close()
			return nil, fmt.Errorf("file_lock_wait: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// WithSpriteAccess binds an existing sprite to a private working copy for a write
// operation. Commit occurs only after fn succeeds. Read operations take the same
// exclusive lock but use the original file without copying or replacing it.
func WithSpriteAccess(ctx context.Context, path string, write bool, fn func(context.Context) error) error {
	return WithFileLocks(ctx, []string{path}, func(ctx context.Context) error {
		canonical, err := CanonicalPath(path)
		if err != nil {
			return err
		}
		s := scopeOf(ctx)
		key := pathKey(canonical)
		if _, ok := s.sprites[key]; ok {
			return fn(ctx)
		}
		run := func(staged string) error {
			child := &fileScope{locked: s.locked, sprites: make(map[string]string, len(s.sprites)+1)}
			for k, v := range s.sprites {
				child.sprites[k] = v
			}
			child.sprites[key] = staged
			return fn(context.WithValue(ctx, fileContextKey{}, child))
		}
		if !s.locked[key] {
			return fmt.Errorf("file_changed: source target changed before operation")
		}
		if !write {
			return run(canonical)
		}
		return stageFile(ctx, path, true, run)
	})
}

// WithOutputFile atomically publishes one explicit output file. All involved
// input/output paths must already be reserved by WithFileLocks.
func WithOutputFile(ctx context.Context, path string, fn func(string) error) error {
	return WithFileLocks(ctx, []string{path}, func(ctx context.Context) error { return stageFile(ctx, path, false, fn) })
}

func boundSprite(ctx context.Context, path string) (string, bool, error) {
	s := scopeOf(ctx)
	if s == nil {
		return "", false, nil
	}
	c, err := CanonicalPath(path)
	if err != nil {
		return "", false, err
	}
	v, ok := s.sprites[pathKey(c)]
	return v, ok, nil
}

type fileSnapshot struct {
	info os.FileInfo
	hash [32]byte
}

func snapshot(path string) (fileSnapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return fileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("not a regular file: %s", path)
	}
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return fileSnapshot{}, err
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return fileSnapshot{info, sum}, nil
}

func stageFile(ctx context.Context, path string, requireExisting bool, fn func(string) error) error {
	return stageFileWithReplace(ctx, path, requireExisting, fn, replaceFile)
}

func stageFileWithReplace(ctx context.Context, path string, requireExisting bool, fn func(string) error, replace func(string, string) error) error {
	original, err := CanonicalPath(path)
	if err != nil {
		return err
	}
	if s := scopeOf(ctx); s == nil || !s.locked[pathKey(original)] {
		return fmt.Errorf("file_changed: target changed before operation")
	}
	before, err := snapshot(original)
	exists := err == nil
	if err != nil && (!os.IsNotExist(err) || requireExisting) {
		return fmt.Errorf("sprite file not found or unreadable: %s: %w", path, err)
	}
	if exists {
		if before.info.Mode().Perm()&0222 == 0 {
			return fmt.Errorf("file is read-only: %s", path)
		}
		if err = checkSingleLink(original, before.info); err != nil {
			return err
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(original), 0755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(filepath.Dir(original), ".pixel-mcp-stage-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	staged := filepath.Join(dir, filepath.Base(original))
	if exists {
		src, e := os.Open(original)
		if e != nil {
			return e
		}
		dst, e := os.OpenFile(staged, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			src.Close()
			return e
		}
		_, e = io.Copy(dst, src)
		src.Close()
		closeErr := dst.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = fn(staged); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Do not publish a removed, empty, symlinked, or non-regular staging file.
	st, err := os.Lstat(staged)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Size() == 0 {
		return fmt.Errorf("invalid staged output")
	}
	after, err := snapshot(staged)
	if err != nil {
		return err
	}
	currentPath, err := CanonicalPath(path)
	if err != nil {
		return err
	}
	if currentPath != original {
		return fmt.Errorf("file_changed: path target changed during operation")
	}
	current, err := snapshot(original)
	if exists {
		if err != nil || !os.SameFile(before.info, current.info) || before.hash != current.hash || before.info.Mode() != current.info.Mode() {
			return fmt.Errorf("file_changed: original changed during operation")
		}
		if err = checkSingleLink(original, current.info); err != nil {
			return err
		}
		if before.hash == after.hash {
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("file_changed: output appeared during operation")
	}
	mode := os.FileMode(0600)
	if exists {
		mode = before.info.Mode().Perm()
	}
	if err = os.Chmod(staged, mode); err != nil {
		return err
	}
	f, err := os.OpenFile(staged, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Same-directory staging guarantees the same filesystem. Never remove the
	// original or fall back to a truncating copy if atomic replacement fails.
	if err = replace(staged, original); err != nil {
		return fmt.Errorf("file_commit_failed: %w", err)
	}
	return nil
}

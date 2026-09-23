package aseprite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// macOS temp roots may have a /var -> /private/var alias. Fault injection
// compares destinations passed to replaceFile, which are canonical paths.
func outputTestDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return path
}

func writeOutputSet(paths []string) error {
	for i, p := range paths {
		if err := os.WriteFile(p, []byte(fmt.Sprintf("new-%d", i)), 0600); err != nil {
			return err
		}
	}
	return nil
}
func requireFileBytes(t *testing.T, path, value string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, value, string(data))
}
func requireNoOutputStages(t *testing.T, dir string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
	require.NoError(t, err)
	require.Empty(t, paths)
}
func TestOutputFilesRollbackOnCommitFailure(t *testing.T) {
	for _, firstExists := range []bool{false, true} {
		t.Run(fmt.Sprint(firstExists), func(t *testing.T) {
			dir := outputTestDir(t)
			paths := []string{filepath.Join(dir, "one.png"), filepath.Join(dir, "two.png")}
			if firstExists {
				require.NoError(t, os.WriteFile(paths[0], []byte("old-one"), 0640))
			}
			require.NoError(t, os.WriteFile(paths[1], []byte("old-two"), 0600))
			err := withOutputFilesReplace(context.Background(), paths, writeOutputSet, func(src, dst string) error {
				if dst == paths[1] {
					return errors.New("injected second publish failure")
				}
				return replaceFile(src, dst)
			})
			require.ErrorContains(t, err, "file_commit_failed")
			if firstExists {
				requireFileBytes(t, paths[0], "old-one")
				st, err := os.Stat(paths[0])
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0640), st.Mode().Perm())
			} else {
				_, err := os.Stat(paths[0])
				require.True(t, os.IsNotExist(err))
			}
			requireFileBytes(t, paths[1], "old-two")
			requireNoOutputStages(t, dir)
			require.NoError(t, WithOutputFiles(context.Background(), paths, writeOutputSet))
			requireFileBytes(t, paths[0], "new-0")
			requireFileBytes(t, paths[1], "new-1")
		})
	}
}
func TestOutputFilesCancelDuringPublication(t *testing.T) {
	dir := outputTestDir(t)
	paths := []string{filepath.Join(dir, "one"), filepath.Join(dir, "two")}
	for _, p := range paths {
		require.NoError(t, os.WriteFile(p, []byte("old"), 0600))
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := withOutputFilesReplace(ctx, paths, writeOutputSet, func(src, dst string) error {
		err := replaceFile(src, dst)
		if !strings.Contains(src, "backup") {
			cancel()
		}
		return err
	})
	require.ErrorIs(t, err, context.Canceled)
	for _, p := range paths {
		requireFileBytes(t, p, "old")
	}
	requireNoOutputStages(t, dir)
}
func TestOutputFilesStageFailurePublishesNothing(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "symlink", "callback", "external-change"} {
		t.Run(kind, func(t *testing.T) {
			dir := outputTestDir(t)
			paths := []string{filepath.Join(dir, "one"), filepath.Join(dir, "two")}
			for _, p := range paths {
				require.NoError(t, os.WriteFile(p, []byte("old"), 0600))
			}
			err := WithOutputFiles(context.Background(), paths, func(staged []string) error {
				require.NoError(t, os.WriteFile(staged[0], []byte("new"), 0600))
				switch kind {
				case "empty":
					return os.WriteFile(staged[1], nil, 0600)
				case "symlink":
					return os.Symlink(paths[1], staged[1])
				case "callback":
					return errors.New("render failure")
				case "external-change":
					require.NoError(t, os.WriteFile(paths[1], []byte("external"), 0600))
					return os.WriteFile(staged[1], []byte("new"), 0600)
				}
				return nil
			})
			require.Error(t, err)
			requireFileBytes(t, paths[0], "old")
			expected := "old"
			if kind == "external-change" {
				expected = "external"
			}
			requireFileBytes(t, paths[1], expected)
			requireNoOutputStages(t, dir)
		})
	}
}
func TestOutputFilesRollbackFailureRetainsBackup(t *testing.T) {
	dir := outputTestDir(t)
	paths := []string{filepath.Join(dir, "one"), filepath.Join(dir, "two")}
	for _, p := range paths {
		require.NoError(t, os.WriteFile(p, []byte("old"), 0600))
	}
	err := withOutputFilesReplace(context.Background(), paths, writeOutputSet, func(src, dst string) error {
		if dst == paths[1] || strings.Contains(src, "backup") {
			return errors.New("injected I/O failure")
		}
		return replaceFile(src, dst)
	})
	require.ErrorContains(t, err, "file_rollback_failed")
	backups, e := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*", ".original-backup"))
	require.NoError(t, e)
	require.Len(t, backups, 2)
	for _, p := range backups {
		requireFileBytes(t, p, "old")
	}
	requireFileBytes(t, paths[0], "new-0")
	requireFileBytes(t, paths[1], "old")
}
func TestOutputFilesRejectAliasesAndReadOnly(t *testing.T) {
	for _, kind := range []string{"duplicate", "symlink-alias", "hardlink", "readonly", "bound-source"} {
		t.Run(kind, func(t *testing.T) {
			dir := outputTestDir(t)
			a := filepath.Join(dir, "a")
			b := filepath.Join(dir, "b")
			require.NoError(t, os.WriteFile(a, []byte("old"), 0600))
			paths := []string{a, b}
			switch kind {
			case "duplicate":
				paths[1] = a
			case "symlink-alias":
				require.NoError(t, os.Symlink(a, b))
			case "hardlink":
				require.NoError(t, os.Link(a, b))
			case "readonly":
				require.NoError(t, os.Chmod(a, 0400))
				defer os.Chmod(a, 0600)
			}
			called := false
			run := func(ctx context.Context) error {
				return WithOutputFiles(ctx, paths, func([]string) error { called = true; return nil })
			}
			var err error
			if kind == "bound-source" {
				err = WithFileLocks(context.Background(), paths, func(ctx context.Context) error { return WithSpriteAccess(ctx, a, false, run) })
			} else {
				err = run(context.Background())
			}
			require.Error(t, err)
			require.False(t, called)
			requireFileBytes(t, a, "old")
			requireNoOutputStages(t, dir)
		})
	}
}
func TestOutputFilesOverlapWaitAndCancellation(t *testing.T) {
	dir := outputTestDir(t)
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithOutputFiles(context.Background(), []string{a, b}, func(paths []string) error { close(ready); <-release; return writeOutputSet(paths) })
	}()
	<-ready
	defer func() { close(release); require.NoError(t, <-done) }()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	called := false
	err := WithOutputFiles(ctx, []string{b, a}, func([]string) error { called = true; return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
}

func TestOutputFilesConcurrentWriterIsNotOverwrittenByRollback(t *testing.T) {
	dir := outputTestDir(t)
	paths := []string{filepath.Join(dir, "one"), filepath.Join(dir, "two")}
	for _, p := range paths {
		require.NoError(t, os.WriteFile(p, []byte("old"), 0600))
	}
	err := withOutputFilesReplace(context.Background(), paths, writeOutputSet, func(src, dst string) error {
		if dst == paths[1] {
			require.NoError(t, os.WriteFile(paths[0], []byte("external writer"), 0600))
			return errors.New("publish failed")
		}
		return replaceFile(src, dst)
	})
	require.ErrorContains(t, err, "file_rollback_failed")
	requireFileBytes(t, paths[0], "external writer")
	requireFileBytes(t, paths[1], "old")
	backups, e := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*", ".original-backup"))
	require.NoError(t, e)
	require.Len(t, backups, 2)
	for _, p := range backups {
		requireFileBytes(t, p, "old")
	}
}

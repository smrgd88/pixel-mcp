package aseprite

import (
	"context"
	"errors"
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFileProtectionCommitAndRollback(t *testing.T) {
	for _, mode := range []string{"commit", "error", "cancel", "empty", "external", "removed", "symlink", "panic"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "sprite.aseprite")
			require.NoError(t, os.WriteFile(path, []byte("original"), 0640))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			run := func() error {
				return WithSpriteAccess(ctx, path, true, func(ctx context.Context) error {
					stage, ok, err := boundSprite(ctx, path)
					require.NoError(t, err)
					require.True(t, ok)
					require.NotEqual(t, path, stage)
					require.NoError(t, os.WriteFile(stage, []byte("changed"), 0600))
					switch mode {
					case "error":
						return errors.New("after saving stage")
					case "cancel":
						cancel()
					case "empty":
						require.NoError(t, os.WriteFile(stage, nil, 0600))
					case "external":
						require.NoError(t, os.WriteFile(path, []byte("external"), 0640))
					case "removed":
						require.NoError(t, os.Remove(stage))
					case "symlink":
						require.NoError(t, os.Remove(stage))
						require.NoError(t, os.Symlink(path, stage))
					case "panic":
						panic("handler failed")
					}
					return nil
				})
			}
			if mode == "panic" {
				require.Panics(t, func() { _ = run() })
			} else {
				err := run()
				if mode == "commit" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
			expected := "original"
			if mode == "commit" {
				expected = "changed"
			}
			if mode == "external" {
				expected = "external"
			}
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, expected, string(data))
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, os.FileMode(0640), info.Mode().Perm())
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, entries, 1)
			require.NoError(t, WithFileLocks(context.Background(), []string{path}, func(context.Context) error { return nil }))
		})
	}
}

func TestFileProtectionCanonicalAndConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "count.aseprite")
	require.NoError(t, os.WriteFile(path, []byte("0"), 0600))
	alias := filepath.Join(dir, "alias.aseprite")
	require.NoError(t, os.Symlink(path, alias))
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := path
			if i%2 == 0 {
				p = alias
			}
			errs <- WithSpriteAccess(context.Background(), p, true, func(ctx context.Context) error {
				staged, _, err := boundSprite(ctx, p)
				if err != nil {
					return err
				}
				b, err := os.ReadFile(staged)
				if err != nil {
					return err
				}
				var n int
				fmt.Sscanf(string(b), "%d", &n)
				return os.WriteFile(staged, []byte(fmt.Sprint(n+1)), 0600)
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "12", string(b))
	st, err := os.Lstat(alias)
	require.NoError(t, err)
	require.NotZero(t, st.Mode()&os.ModeSymlink)
}

func TestFileProtectionLockCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sprite.aseprite")
	ready := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithFileLocks(context.Background(), []string{path}, func(context.Context) error { close(ready); <-release; return nil })
	}()
	<-ready
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	called := false
	err := WithFileLocks(ctx, []string{path}, func(context.Context) error { called = true; return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
	// A different file is not blocked by this lock.
	require.NoError(t, WithFileLocks(context.Background(), []string{path + "other"}, func(context.Context) error { return nil }))
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, WithFileLocks(context.Background(), []string{path}, func(context.Context) error { return nil }))
}

func TestFileProtectionReadOnlyAndHardLink(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sprite.aseprite")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0400))
	before, _ := os.Stat(path)
	require.NoError(t, WithSpriteAccess(context.Background(), path, false, func(ctx context.Context) error {
		p, _, e := boundSprite(ctx, path)
		require.Equal(t, path, p)
		return e
	}))
	after, _ := os.Stat(path)
	require.True(t, os.SameFile(before, after))
	require.Error(t, WithSpriteAccess(context.Background(), path, true, func(context.Context) error { return nil }))
	require.NoError(t, os.Chmod(path, 0600))
	require.NoError(t, os.Link(path, path+"alias"))
	require.Error(t, WithSpriteAccess(context.Background(), path, true, func(context.Context) error { return nil }))
}

func TestFileProtectionNewOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.aseprite")
	require.NoError(t, WithOutputFile(context.Background(), path, func(stage string) error { return os.WriteFile(stage, []byte("published"), 0600) }))
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "published", string(b))
}

func TestFileProtectionProcessDeath(t *testing.T) {
	if path := os.Getenv("PIXEL_MCP_LOCK_CHILD"); path != "" {
		err := WithSpriteAccess(context.Background(), path, true, func(ctx context.Context) error {
			staged, _, err := boundSprite(ctx, path)
			if err != nil {
				return err
			}
			if err = os.WriteFile(staged, []byte("partial"), 0600); err != nil {
				return err
			}
			if err = os.WriteFile(path+".ready", []byte("ready"), 0600); err != nil {
				return err
			}
			for {
				time.Sleep(time.Hour)
			}
		})
		if err != nil {
			os.Exit(2)
		}
		return
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "source.aseprite")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0600))
	cmd := exec.Command(os.Args[0], "-test.run=^TestFileProtectionProcessDeath$")
	cmd.Env = append(os.Environ(), "PIXEL_MCP_LOCK_CHILD="+path)
	require.NoError(t, cmd.Start())
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case <-deadline:
			t.Fatal("child did not acquire lock")
		case <-ticker.C:
			_, err := os.Stat(path + ".ready")
			ready = err == nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	require.ErrorIs(t, WithFileLocks(ctx, []string{path}, func(context.Context) error { return nil }), context.DeadlineExceeded)
	cancel()
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, WithFileLocks(ctx, []string{path}, func(context.Context) error { return nil }))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(data))
}

func TestFileProtectionCommitFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sprite.aseprite")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0600))
	err := WithFileLocks(context.Background(), []string{path}, func(ctx context.Context) error {
		return stageFileWithReplace(ctx, path, true, func(staged string) error {
			return os.WriteFile(staged, []byte("complete new bytes"), 0600)
		}, func(src, dst string) error {
			require.Equal(t, path, dst)
			require.FileExists(t, src)
			return os.ErrPermission
		})
	})
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorContains(t, err, "file_commit_failed")
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(b))
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.NoError(t, WithFileLocks(context.Background(), []string{path}, func(context.Context) error { return nil }))
}

func TestFileProtectionSymlinkParentTraversal(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "physical", "child"), 0700))
	require.NoError(t, os.Symlink(filepath.Join(dir, "physical", "child"), filepath.Join(dir, "link")))
	actual := filepath.Join(dir, "physical", "sprite.aseprite")
	decoy := filepath.Join(dir, "sprite.aseprite")
	require.NoError(t, os.WriteFile(actual, []byte("original"), 0600))
	require.NoError(t, os.WriteFile(decoy, []byte("decoy"), 0600))
	input := dir + string(filepath.Separator) + "link" + string(filepath.Separator) + ".." + string(filepath.Separator) + "sprite.aseprite"
	canonical, err := CanonicalPath(input)
	require.NoError(t, err)
	require.Equal(t, actual, canonical)
	output := dir + "/link/../new.aseprite"
	canonical, err = CanonicalPath(output)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "physical", "new.aseprite"), canonical)
	require.NoError(t, WithSpriteAccess(context.Background(), input, true, func(ctx context.Context) error {
		stage, _, err := boundSprite(ctx, input)
		if err != nil {
			return err
		}
		return os.WriteFile(stage, []byte("changed"), 0600)
	}))
	data, err := os.ReadFile(actual)
	require.NoError(t, err)
	require.Equal(t, "changed", string(data))
	data, err = os.ReadFile(decoy)
	require.NoError(t, err)
	require.Equal(t, "decoy", string(data))
}

func TestFileProtectionOutputRequiresNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "export.png")
	require.NoError(t, os.WriteFile(path, []byte("old output"), 0600))
	err := WithOutputFile(context.Background(), path, func(stage string) error {
		// A multi-file exporter can write numbered outputs without creating the
		// requested base file. The old output must not stand in for a new one.
		return os.WriteFile(stage+"1.png", []byte("new frame"), 0600)
	})
	require.Error(t, err)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "old output", string(b))
}

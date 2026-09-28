package aseprite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func snapshotFixture(t *testing.T) (*SnapshotStore, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sprite.aseprite")
	require.NoError(t, os.WriteFile(p, []byte("original bytes"), 0640))
	return NewSnapshotStore(filepath.Join(dir, "store")), p
}
func TestSnapshotRoundTripRestartAndBackup(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	before, _ := os.Stat(p)
	first, err := s.Create(ctx, p, "initial")
	require.NoError(t, err)
	after, _ := os.Stat(p)
	require.True(t, os.SameFile(before, after))
	require.Equal(t, before.ModTime(), after.ModTime())
	require.NoError(t, os.WriteFile(p, []byte("edited bytes"), 0640))
	s = NewSnapshotStore(s.Dir)
	backup, err := s.Restore(ctx, p, first.ID)
	require.NoError(t, err)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "original bytes", string(b))
	info, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0640), info.Mode().Perm())
	_, err = s.Restore(ctx, p, backup.ID)
	require.NoError(t, err)
	b, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "edited bytes", string(b))
	items, err := s.List(ctx, p)
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.NoError(t, s.Delete(ctx, first.ID))
	_, err = s.Restore(ctx, p, first.ID)
	require.Error(t, err)
}
func TestSnapshotCapacityExpiryAndOrphans(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	s.MaxCount = 1
	first, err := s.Create(ctx, p, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("edited"), 0600))
	_, err = s.Restore(ctx, p, first.ID)
	require.ErrorContains(t, err, "snapshot_capacity")
	b, _ := os.ReadFile(p)
	require.Equal(t, "edited", string(b))
	items, err := s.List(ctx, "")
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, s.Delete(ctx, first.ID))
	s.MaxBytes = 2
	_, err = s.Create(ctx, p, "")
	require.ErrorContains(t, err, "snapshot_capacity")
	s.MaxBytes = 100
	s.TTL = time.Millisecond
	_, err = s.Create(ctx, p, "")
	require.NoError(t, err)
	time.Sleep(3 * time.Millisecond)
	items, err = s.List(ctx, "")
	require.NoError(t, err)
	require.Empty(t, items)
	orphan := filepath.Join(s.Dir, ".pending-"+first.ID)
	require.NoError(t, os.Mkdir(orphan, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(orphan, "partial"), []byte("x"), 0600))
	_, err = s.List(ctx, "")
	require.NoError(t, err)
	_, err = os.Stat(orphan)
	require.True(t, os.IsNotExist(err))
}
func TestSnapshotRejectsTamperingAndUnsafePaths(t *testing.T) {
	for _, kind := range []string{"checksum", "metadata", "symlink", "wrong-source", "traversal", "read-only", "hard-link", "missing"} {
		t.Run(kind, func(t *testing.T) {
			s, p := snapshotFixture(t)
			ctx := context.Background()
			first, err := s.Create(ctx, p, "")
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(p, []byte("edited bytes"), 0640))
			id := first.ID
			target := p
			switch kind {
			case "checksum":
				require.NoError(t, os.WriteFile(filepath.Join(s.Dir, id, "sprite"), []byte("corrupt!!bytes!"), 0600))
			case "metadata":
				require.NoError(t, os.WriteFile(filepath.Join(s.Dir, id, "metadata.json"), []byte("{}"), 0600))
			case "symlink":
				require.NoError(t, os.Remove(filepath.Join(s.Dir, id, "sprite")))
				require.NoError(t, os.Symlink(p, filepath.Join(s.Dir, id, "sprite")))
			case "wrong-source":
				target = filepath.Join(t.TempDir(), "other")
				require.NoError(t, os.WriteFile(target, []byte("other"), 0600))
			case "traversal":
				id = "../" + id
			case "read-only":
				require.NoError(t, os.Chmod(p, 0400))
			case "hard-link":
				require.NoError(t, os.Link(p, filepath.Join(t.TempDir(), "alias")))
			case "missing":
				require.NoError(t, os.Remove(p))
			}
			_, err = s.Restore(ctx, target, id)
			require.Error(t, err)
			if kind != "missing" {
				b, e := os.ReadFile(p)
				require.NoError(t, e)
				require.Equal(t, "edited bytes", string(b))
			}
		})
	}
}
func TestSnapshotConcurrentCapacityAndCancellation(t *testing.T) {
	s, p := snapshotFixture(t)
	s.MaxCount = 2
	var wg sync.WaitGroup
	var mu sync.Mutex
	success := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Create(context.Background(), p, "")
			if err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			} else {
				require.Contains(t, err.Error(), "snapshot_capacity")
			}
		}()
	}
	wg.Wait()
	require.Equal(t, 2, success)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.List(ctx, "")
	require.ErrorIs(t, err, context.Canceled)
	items, err := s.List(context.Background(), "")
	require.NoError(t, err)
	require.Len(t, items, 2)
	entries, err := os.ReadDir(s.Dir)
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, strings.HasPrefix(e.Name(), ".pending-"))
	}
}
func TestSnapshotStorageBoundary(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	require.NoError(t, os.Mkdir(s.Dir, 0755))
	_, err := s.Create(ctx, p, "")
	require.ErrorContains(t, err, "private")
	require.NoError(t, os.Chmod(s.Dir, 0700))
	q := filepath.Join(s.Dir, "source")
	require.NoError(t, os.WriteFile(q, []byte("x"), 0600))
	_, err = s.Create(ctx, q, "")
	require.ErrorContains(t, err, "outside")
	require.NoError(t, os.Remove(q))
	outside := t.TempDir()
	require.NoError(t, os.Remove(s.Dir))
	require.NoError(t, os.Symlink(outside, s.Dir))
	_, err = s.Create(ctx, p, "")
	require.Error(t, err)
}

func TestSnapshotSymlinkReadOnlyAndMissingFilter(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	alias := filepath.Join(t.TempDir(), "alias.aseprite")
	require.NoError(t, os.Symlink(p, alias))
	require.NoError(t, os.Chmod(p, 0400))
	item, err := s.Create(ctx, alias, "")
	require.NoError(t, err)
	canonical, err := CanonicalPath(p)
	require.NoError(t, err)
	require.Equal(t, canonical, item.SpritePath)
	items, err := s.List(ctx, alias)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, os.Remove(p))
	items, err = s.List(ctx, p)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestSnapshotCommitFailurePreservesOriginalAndBackup(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	first, err := s.Create(ctx, p, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("edited"), 0640))
	backup, err := s.restoreWithReplace(ctx, p, first.ID, func(_, _ string) error { return os.ErrPermission })
	require.ErrorContains(t, err, "file_commit_failed")
	require.NotEmpty(t, backup.ID)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "edited", string(b))
	items, err := s.List(ctx, p)
	require.NoError(t, err)
	require.Len(t, items, 2)
	b, err = os.ReadFile(filepath.Join(s.Dir, backup.ID, "sprite"))
	require.NoError(t, err)
	require.Equal(t, "edited", string(b))
	entries, err := os.ReadDir(filepath.Dir(p))
	require.NoError(t, err)
	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), ".pixel-mcp-stage-"))
	}
}
func TestSnapshotHashLimitsAndCancellation(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := snapshotSource(ctx, p, s.MaxBytes)
	require.ErrorIs(t, err, context.Canceled)
	_, err = snapshotSource(context.Background(), p, 2)
	require.ErrorContains(t, err, "snapshot_capacity")
	_, err = s.Create(ctx, p, "")
	require.ErrorIs(t, err, context.Canceled)
	_, err = os.Stat(s.Dir)
	require.True(t, os.IsNotExist(err), "cancelled call should not create store")
}

func TestSnapshotStorageSymlinkWithTrailingSeparator(t *testing.T) {
	s, p := snapshotFixture(t)
	require.NoError(t, os.Symlink(t.TempDir(), s.Dir))
	s.Dir += string(os.PathSeparator)
	_, err := s.Create(context.Background(), p, "")
	require.Error(t, err, "a trailing separator must not bypass the store symlink check")
}

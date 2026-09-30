//go:build linux

package aseprite

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSnapshotRestoreAcrossFilesystems(t *testing.T) {
	s, p := snapshotFixture(t)
	dir, err := os.MkdirTemp("/dev/shm", "pixel-mcp-snapshot-")
	if err != nil {
		t.Skipf("separate tmpfs unavailable: %v", err)
	}
	defer os.RemoveAll(dir)
	sourceInfo, err := os.Stat(p)
	require.NoError(t, err)
	storeInfo, err := os.Stat(dir)
	require.NoError(t, err)
	if sourceInfo.Sys().(*syscall.Stat_t).Dev == storeInfo.Sys().(*syscall.Stat_t).Dev {
		t.Skip("separate filesystems unavailable")
	}
	s.Dir = filepath.Join(dir, "store")
	item, err := s.Create(context.Background(), p, "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p, []byte("changed"), 0640))
	_, err = s.Restore(context.Background(), p, item.ID)
	require.NoError(t, err)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "original bytes", string(b))
}

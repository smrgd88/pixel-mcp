//go:build linux || darwin

package aseprite

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestFileProtectionRejectsFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe.aseprite")
	require.NoError(t, unix.Mkfifo(path, 0600))
	called := false
	require.ErrorContains(t, WithSpriteAccess(context.Background(), path, false, func(context.Context) error { called = true; return nil }), "not a regular file")
	require.False(t, called)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- WithSpriteAccess(ctx, path, true, func(context.Context) error { t.Error("must not run for FIFO"); return nil })
	}()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "not a regular file")
	case <-time.After(500 * time.Millisecond):
		// Release a buggy blocking Open without leaving a goroutine behind.
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0)
		require.NoError(t, err)
		require.NoError(t, unix.Close(fd))
		<-done
		t.Fatal("non-regular input blocked despite request deadline")
	}
	require.NoError(t, WithFileLocks(context.Background(), []string{path}, func(context.Context) error { return nil }))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeNamedPipe)
}

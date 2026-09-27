package aseprite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSpritePreviewIsolatedAndCleaned(t *testing.T) {
	for _, kind := range []string{"success", "error", "cancel", "panic", "external-change"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "source.aseprite")
			require.NoError(t, os.WriteFile(p, []byte("source"), 0600))
			info, err := os.Stat(p)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var private string
			run := func() error {
				return WithSpritePreview(ctx, p, func(ctx context.Context, copyPath string) error {
					private = copyPath
					require.NotEqual(t, p, copyPath)
					bound, ok, err := boundSprite(ctx, p)
					require.NoError(t, err)
					require.True(t, ok)
					require.Equal(t, copyPath, bound)
					requireFileBytes(t, copyPath, "source")
					require.NoError(t, os.WriteFile(copyPath, []byte("simulated change"), 0600))
					requireFileBytes(t, p, "source")
					switch kind {
					case "error":
						return errors.New("simulation failed")
					case "cancel":
						cancel()
					case "panic":
						panic("simulation panic")
					case "external-change":
						require.NoError(t, os.WriteFile(p, []byte("external"), 0600))
					}
					return nil
				})
			}
			if kind == "panic" {
				require.Panics(t, func() { _ = run() })
			} else {
				err := run()
				if kind == "success" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				if kind == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				}
				if kind == "external-change" {
					require.ErrorContains(t, err, "file_changed")
				}
			}
			expected := "source"
			if kind == "external-change" {
				expected = "external"
			}
			requireFileBytes(t, p, expected)
			after, err := os.Stat(p)
			require.NoError(t, err)
			require.True(t, os.SameFile(info, after))
			require.NotEmpty(t, private)
			_, err = os.Stat(filepath.Dir(private))
			require.True(t, os.IsNotExist(err))
			require.NoError(t, WithFileLocks(context.Background(), []string{p}, func(context.Context) error { return nil }))
		})
	}
}
func TestSpritePreviewReadOnlyAndHardLinkedSource(t *testing.T) {
	p := filepath.Join(t.TempDir(), "source.aseprite")
	require.NoError(t, os.WriteFile(p, []byte("original"), 0400))
	defer os.Chmod(p, 0600)
	alias := filepath.Join(t.TempDir(), "alias.aseprite")
	require.NoError(t, os.Link(p, alias))
	require.NoError(t, WithSpritePreview(context.Background(), alias, func(ctx context.Context, copyPath string) error {
		requireFileBytes(t, p, "original")
		return os.WriteFile(copyPath, []byte("preview"), 0600)
	}))
	requireFileBytes(t, p, "original")
	requireFileBytes(t, alias, "original")
	info, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0400), info.Mode().Perm())
}
func TestSpritePreviewLocksSourceAndRejectsNestedBinding(t *testing.T) {
	p := filepath.Join(t.TempDir(), "source.aseprite")
	require.NoError(t, os.WriteFile(p, []byte("source"), 0600))
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithSpritePreview(context.Background(), p, func(ctx context.Context, _ string) error {
			err := WithSpritePreview(ctx, p, func(context.Context, string) error { return nil })
			if err == nil {
				return errors.New("nested preview accepted")
			}
			close(ready)
			<-release
			return nil
		})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("preview setup failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("preview did not acquire source lock")
	}
	defer func() { close(release); require.NoError(t, <-done) }()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	called := false
	err := WithSpriteAccess(ctx, p, true, func(context.Context) error { called = true; return nil })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
	requireFileBytes(t, p, "source")
}

package aseprite

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"os"
	"path/filepath"
	"testing"
)

func TestSpriteRevisionUsesBoundCopy(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sprite.aseprite")
	require.NoError(t, os.WriteFile(p, []byte("original"), 0600))
	var revision string
	require.NoError(t, WithSpriteAccess(context.Background(), p, false, func(ctx context.Context) error {
		var err error
		revision, err = SpriteRevision(ctx, p)
		return err
	}))
	err := WithSpriteAccess(context.Background(), p, true, func(ctx context.Context) error {
		// An uncooperative writer changes the original after staging. The mutation
		// must compare the bytes it will load, not freshly hash the original path.
		require.NoError(t, os.WriteFile(p, []byte("external"), 0600))
		actual, e := SpriteRevision(ctx, p)
		require.NoError(t, e)
		require.Equal(t, revision, actual)
		stage, _, e := boundSprite(ctx, p)
		require.NoError(t, e)
		return os.WriteFile(stage, []byte("edited"), 0600)
	})
	require.ErrorContains(t, err, "file_changed")
	actual, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "external", string(actual))
	_, err = SpriteRevision(context.Background(), p)
	require.Equal(t, "file_lock_scope", diagnostics.Classify(err).Code)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = SpriteRevision(ctx, p)
	require.ErrorIs(t, err, context.Canceled)
}

//go:build integration

package aseprite

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
)

func TestIntegrationCapabilities(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	c := NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	caps, err := c.CheckCapabilities(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, caps.Version)
	require.GreaterOrEqual(t, caps.APIVersion, MinimumAPIVersion)
	out, err := c.ExecuteLua(context.Background(), `print("PAYLOAD_ONLY")`, "")
	require.NoError(t, err)
	require.Equal(t, "PAYLOAD_ONLY\n", out)

	// Exercise rejection using the real executable and a deliberately higher
	// internal test floor. No mock executable or bypass is exposed to callers.
	path := filepath.Join(t.TempDir(), "original.aseprite")
	_, err = c.ExecuteLua(context.Background(), NewLuaGenerator().CreateCanvas(8, 8, ColorModeRGB, path), "")
	require.NoError(t, err)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, floor := range []struct {
		version string
		api     int
	}{{"9999.0.0", MinimumAPIVersion}, {MinimumVersion, caps.APIVersion + 1}} {
		out, err = c.executeLuaWithRequirements(context.Background(), `app.activeSprite:resize(1,1); app.activeSprite:saveAs(app.activeSprite.filename)`, path, floor.version, floor.api)
		var ce *CapabilityError
		require.ErrorAs(t, err, &ce)
		require.Equal(t, "unsupported_aseprite", ce.Code)
		require.Empty(t, out)
		after, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		require.Equal(t, before, after)
	}
	// A rejected check must not poison later calls; the same real payload runs
	// after the normal supported-floor probe.
	_, err = c.ExecuteLua(context.Background(), `app.activeSprite:resize(1,1); app.activeSprite:saveAs(app.activeSprite.filename)`, path)
	require.NoError(t, err)
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
	files, err := os.ReadDir(c.tempDir)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestIntegrationCapabilitiesConcurrentAndCancel(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	c := NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := c.CheckCapabilities(context.Background()); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.ExecuteLua(ctx, `print("must not run")`, "")
	var ce *CapabilityError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, "capability_probe_failed", ce.Code)
	require.ErrorIs(t, err, context.Canceled)
	c.timeout = time.Nanosecond
	_, err = c.CheckCapabilities(context.Background())
	require.ErrorAs(t, err, &ce)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	files, err := os.ReadDir(c.tempDir)
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestIntegrationCapabilitiesRunningCancel(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	c := NewClient(cfg.AsepritePath, t.TempDir(), cfg.Timeout)
	marker := filepath.Join(t.TempDir(), "started.aseprite")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := c.ExecuteLua(ctx, NewLuaGenerator().CreateCanvas(1, 1, ColorModeRGB, marker)+"\nwhile true do end", "")
		done <- err
	}()
	// Wait for the real payload to start, then cancel a running process.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			t.Fatalf("payload exited before cancellation: %v", err)
		case <-ctx.Done():
			t.Fatal("payload did not start before test deadline")
		case <-ticker.C:
			if _, err := os.Stat(marker); err != nil {
				continue
			}
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("canceled process did not exit")
			}
			files, err := os.ReadDir(c.tempDir)
			require.NoError(t, err)
			require.Empty(t, files)
			return
		}
	}
}

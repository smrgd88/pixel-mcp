//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/mtlog"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/internal/testutil"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestHealthCapabilityJSON(t *testing.T) {
	cfg := testutil.LoadTestConfig(t)
	logger := mtlog.New(mtlog.WithMinimumLevel(core.FatalLevel))
	var buf bytes.Buffer
	require.Equal(t, 0, writeHealthCheck(cfg, logger, &buf))
	var result healthResult
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.True(t, result.Success)
	require.NotEmpty(t, result.AsepriteVersion)
	require.GreaterOrEqual(t, result.APIVersion, aseprite.MinimumAPIVersion)
	require.Equal(t, aseprite.MinimumVersion, result.MinimumVersion)
	require.Empty(t, result.ErrorCode)
	cfg.AsepritePath = "/nonexistent/aseprite"
	buf.Reset()
	require.Equal(t, 1, writeHealthCheck(cfg, logger, &buf))
	result = healthResult{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
	require.False(t, result.Success)
	require.Equal(t, "capability_probe_failed", result.ErrorCode)
}

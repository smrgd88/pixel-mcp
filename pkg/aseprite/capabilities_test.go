package aseprite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCapabilitySupportFloor(t *testing.T) {
	for _, tt := range []struct {
		version string
		api     int
		code    string
	}{
		{"1.3.17.2", 39, ""}, {"1.3.18.3-dev", 39, ""}, {"1.4.0", 40, ""}, {"2.0.0", 100, ""},
		{"1.3.17.1", 39, "unsupported_aseprite"}, {"1.3.17", 39, "unsupported_aseprite"},
		{"1.3.9", 50, "unsupported_aseprite"}, {"1.3.17.2-beta1", 39, "unsupported_aseprite"},
		{"1.3.17.2-dev", 39, "unsupported_aseprite"}, {"1.3.18.3", 38, "unsupported_aseprite"},
		{"garbage", 39, "capability_probe_failed"}, {"1.3.17.2.1", 39, "capability_probe_failed"},
	} {
		t.Run(tt.version, func(t *testing.T) {
			err := validateCapabilities(Capabilities{tt.version, tt.api}, MinimumVersion, MinimumAPIVersion)
			if tt.code == "" {
				require.NoError(t, err)
				return
			}
			var ce *CapabilityError
			require.ErrorAs(t, err, &ce)
			require.Equal(t, tt.code, ce.Code)
		})
	}
}

func TestParseCapabilities(t *testing.T) {
	valid := `PIXEL_MCP_CAPABILITIES={"aseprite_version":"1.3.18.3-dev","api_version":39}`
	caps, err := parseCapabilities("noise\n" + valid + "\r\n")
	require.NoError(t, err)
	require.Equal(t, Capabilities{"1.3.18.3-dev", 39}, caps)
	for _, invalid := range []string{"", "1.3.18.3", valid + "\n" + valid,
		`PIXEL_MCP_CAPABILITIES={}`, `PIXEL_MCP_CAPABILITIES=null`,
		`PIXEL_MCP_CAPABILITIES={"aseprite_version":"1.3.18.3"}`,
		`PIXEL_MCP_CAPABILITIES={"aseprite_version":"1.3.18.3","api_version":"39"}`,
		`PIXEL_MCP_CAPABILITIES={"aseprite_version":"1.3.18.3","api_version":39.5}`,
		`PIXEL_MCP_CAPABILITIES={"aseprite_version":"unknown","api_version":39}`,
	} {
		t.Run(invalid, func(t *testing.T) { _, err := parseCapabilities(invalid); require.Error(t, err) })
	}
}

func TestCapabilityProbeFailure(t *testing.T) {
	c := NewClient("/nonexistent/pixel-mcp-aseprite", t.TempDir(), time.Second)
	_, err := c.ExecuteLua(context.Background(), `error("tool code must not run")`, "")
	var ce *CapabilityError
	require.ErrorAs(t, err, &ce)
	require.Equal(t, "capability_probe_failed", ce.Code)
	require.NotNil(t, errors.Unwrap(ce))
}

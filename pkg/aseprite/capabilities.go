package aseprite

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// MinimumVersion and MinimumAPIVersion define the supported Aseprite baseline.
const MinimumVersion = "1.3.17.2"
const MinimumAPIVersion = 39

// Capabilities identifies the executable that actually ran the Lua probe.
type Capabilities struct {
	Version    string `json:"aseprite_version"`
	APIVersion int    `json:"api_version"`
}

// CapabilityError distinguishes an unsupported runtime from a failed probe.
// Code is stable; Cause preserves process errors for diagnostic use.
type CapabilityError struct {
	Code  string
	Cause error
}

func (e *CapabilityError) Error() string { return e.Code + ": " + e.Cause.Error() }
func (e *CapabilityError) Unwrap() error { return e.Cause }

const capabilityMarker = "PIXEL_MCP_CAPABILITIES="
const capabilityProbe = `print("PIXEL_MCP_CAPABILITIES=" .. json.encode({aseprite_version=tostring(app.version), api_version=app.apiVersion}))`

// GetCapabilities probes Aseprite without opening a sprite or executing tool code.
// It does not reject unsupported versions, so health can report their values.
func (c *Client) GetCapabilities(ctx context.Context) (Capabilities, error) {
	out, err := c.executeLuaUnchecked(ctx, capabilityProbe, "")
	if err != nil {
		return Capabilities{}, &CapabilityError{Code: "capability_probe_failed", Cause: err}
	}
	caps, err := parseCapabilities(out)
	if err != nil {
		return Capabilities{}, &CapabilityError{Code: "capability_probe_failed", Cause: err}
	}
	return caps, nil
}

func parseCapabilities(out string) (Capabilities, error) {
	var caps Capabilities
	found := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, capabilityMarker) {
			continue
		}
		if found {
			return caps, fmt.Errorf("duplicate capability response")
		}
		found = true
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, capabilityMarker)), &caps); err != nil {
			return caps, fmt.Errorf("invalid capability response: %w", err)
		}
	}
	if !found || caps.Version == "" || caps.APIVersion <= 0 {
		return caps, fmt.Errorf("missing version or API version in capability response")
	}
	if _, _, err := parseAsepriteVersion(caps.Version); err != nil {
		return caps, err
	}
	return caps, nil
}

// CheckCapabilities checks both the release and API support floors.
func (c *Client) CheckCapabilities(ctx context.Context) (Capabilities, error) {
	caps, err := c.GetCapabilities(ctx)
	if err != nil {
		return caps, err
	}
	return caps, validateCapabilities(caps, MinimumVersion, MinimumAPIVersion)
}

// Aseprite uses up to four numeric components and optional prerelease suffixes.
var asepriteVersionPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:\.(\d+))?(-[0-9A-Za-z][0-9A-Za-z.-]*)?$`)

func parseAsepriteVersion(version string) ([4]int, bool, error) {
	var parts [4]int
	m := asepriteVersionPattern.FindStringSubmatch(version)
	if m == nil {
		return parts, false, fmt.Errorf("invalid Aseprite version %q", version)
	}
	for i := range parts {
		if m[i+1] == "" {
			continue
		}
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return parts, false, fmt.Errorf("invalid Aseprite version %q", version)
		}
		parts[i] = n
	}
	return parts, m[5] != "", nil
}

func validateCapabilities(caps Capabilities, minimum string, minimumAPI int) error {
	actual, prerelease, err := parseAsepriteVersion(caps.Version)
	if err != nil {
		return &CapabilityError{Code: "capability_probe_failed", Cause: err}
	}
	floor, _, err := parseAsepriteVersion(minimum)
	if err != nil {
		return err
	}
	comparison := 0
	for i := range actual {
		if actual[i] < floor[i] {
			comparison = -1
			break
		}
		if actual[i] > floor[i] {
			comparison = 1
			break
		}
	}
	if comparison < 0 || (comparison == 0 && prerelease) || caps.APIVersion < minimumAPI {
		return &CapabilityError{Code: "unsupported_aseprite", Cause: fmt.Errorf("requires Aseprite >= %s and API >= %d; detected %s (API %d)", minimum, minimumAPI, caps.Version, caps.APIVersion)}
	}
	return nil
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"github.com/stretchr/testify/require"
)

func TestExportOptionsDimensionBudget(t *testing.T) {
	for _, layout := range []string{"", "horizontal", "vertical", "rows", "columns", "packed"} {
		require.NoError(t, validateExportDimensions(exportPlan{Width: 16, Height: 16, First: 1, Last: 3}, layout, 2, 3, 1, true))
		for _, p := range []exportPlan{{Width: 65535, Height: 65535, First: 1, Last: 2}, {Width: 1 << 62, Height: 2, First: 1, Last: 1}, {Width: 1, Height: 1, First: 1, Last: 1 << 62}} {
			require.Error(t, validateExportDimensions(p, layout, 0, 0, 0, false))
		}
	}
	require.Error(t, validateExportDimensions(exportPlan{Width: 32767, Height: 1, First: 1, Last: 1}, "horizontal", 1, 0, 0, false))
	require.Error(t, validateExportDimensions(exportPlan{Width: 8192, Height: 8192, First: 1, Last: 2}, "horizontal", 0, 0, 0, false))
}

func TestExportOptionsRangePathsProperties(t *testing.T) {
	// Deterministic bounded property coverage: output names are unique and source
	// numbers never rebase when a selected interval starts after frame one.
	require.NoError(t, quick.Check(func(a, b uint8) bool {
		first := int(a) + 1
		last := first + int(b)
		paths, frames := exportRangePaths("walk007.png", first, last)
		if len(paths) != int(b)+1 || len(frames) != len(paths) {
			return false
		}
		seen := map[string]bool{}
		for i, p := range paths {
			if frames[i] != first+i || seen[p] {
				return false
			}
			seen[p] = true
			if first == last {
				if p != "walk007.png" {
					return false
				}
			} else if p != fmt.Sprintf("walk007_%04d.png", first+i) {
				return false
			}
		}
		return true
	}, &quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(5))}))
}

func TestExportOptionsNativeJSONControlProperties(t *testing.T) {
	check := func(name string) bool {
		name = strings.ToValidUTF8(name, "\ufffd")
		raw := []byte("{\"name\":\"" + strings.ReplaceAll(strings.ReplaceAll(name, `\`, `\\`), `"`, `\"`) + "\"}\n")
		var out struct{ Name string }
		if json.Unmarshal(escapeNativeJSONControls(raw), &out) != nil || out.Name != name {
			return false
		}
		canonical, _ := json.Marshal(out)
		return string(escapeNativeJSONControls(canonical)) == string(canonical)
	}
	all := []byte{'"', '\\'}
	for i := 0; i < 32; i++ {
		all = append(all, byte(i))
	}
	require.True(t, check(string(all)))
	require.NoError(t, quick.Check(check, &quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(5))}))
	require.False(t, json.Valid(escapeNativeJSONControls([]byte(`{"x": invalid}`))))
}

func TestExportOptionsMetadataValidation(t *testing.T) {
	good := `{"frames":{"source 0":{"frame":{"x":0,"y":0,"w":2,"h":3},"duration":200,"sourceSize":{"w":16,"h":16},"spriteSourceSize":{"x":3,"y":5,"w":2,"h":3}}},"meta":{"size":{"w":2,"h":3},"image":"private.png","frameTags":[{"name":"tag","from":0,"to":2,"direction":"reverse"}]}}`
	p := exportPlan{First: 2, Last: 2, Width: 16, Height: 16, Durations: []int{200}}
	for _, tc := range []struct {
		name, text string
		ok         bool
	}{
		{"valid", good, true}, {"duration", strings.Replace(good, `"duration":200`, `"duration":100`, 1), false},
		{"bounds", strings.Replace(good, `"x":0`, `"x":2`, 1), false},
		{"source", strings.Replace(good, `"w":16`, `"w":17`, 1), false},
		{"empty", strings.Replace(good, `"w":2`, `"w":0`, 1), false},
		{"missing", `{"frames":{},"meta":{}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sheet.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.text), 0600))
			err := validateSheetMetadata(path, "published.png", p, 2, 3)
			if !tc.ok {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), `"source_frame_number": 2`)
			require.Contains(t, string(data), `"image": "published.png"`)
			require.Contains(t, string(data), `"from": 0`)
			require.Contains(t, string(data), `"to": 0`)
		})
	}
}

func TestExportOptionsOverwriteRecheckedAfterStaging(t *testing.T) {
	for _, overwrite := range []bool{false, true} {
		t.Run(fmt.Sprint(overwrite), func(t *testing.T) {
			dir := t.TempDir()
			paths := []string{filepath.Join(dir, "sheet.png"), filepath.Join(dir, "sheet.json")}
			require.NoError(t, checkExportOverwrite(paths, &overwrite))
			// Non-cooperating writer wins the gap between the first check and staging.
			require.NoError(t, os.WriteFile(paths[1], []byte("external"), 0640))
			called := false
			err := withExportOutputFiles(context.Background(), paths, &overwrite, func(staged []string) error {
				called = true
				for _, p := range staged {
					if err := os.WriteFile(p, []byte("generated"), 0600); err != nil {
						return err
					}
				}
				return nil
			})
			actual, e := os.ReadFile(paths[1])
			require.NoError(t, e)
			if overwrite {
				require.NoError(t, err)
				require.True(t, called)
				require.Equal(t, "generated", string(actual))
			} else {
				require.ErrorContains(t, err, "overwrite=false")
				require.False(t, called)
				require.Equal(t, "external", string(actual))
				_, e = os.Stat(paths[0])
				require.True(t, os.IsNotExist(e))
			}
			dirs, e := filepath.Glob(filepath.Join(dir, ".pixel-mcp-stage-*"))
			require.NoError(t, e)
			require.Empty(t, dirs)
		})
	}
}

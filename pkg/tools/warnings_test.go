package tools

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOperationWarningConditions(t *testing.T) {
	for _, tt := range []struct {
		name     string
		warnings []ToolWarning
		codes    []string
	}{
		{"quantization without conversion", quantizationWarnings(false, false), []string{"palette_quantization"}},
		{"quantization with conversion", quantizationWarnings(true, false), []string{"palette_quantization", "color_mode_conversion"}},
		{"dither without conversion", quantizationWarnings(false, true), []string{"palette_quantization", "layer_flattening"}},
		{"dither with conversion", quantizationWarnings(true, true), []string{"palette_quantization", "color_mode_conversion", "layer_flattening"}},
		{"flatten", flattenWarnings(), []string{"layer_flattening"}},
		{"nearest", scaleWarnings("nearest", 2, 2), nil},
		{"default", scaleWarnings("", 2, 2), nil},
		{"bilinear", scaleWarnings("bilinear", 2, 2), []string{"resampling"}},
		{"rotsprite", scaleWarnings("rotsprite", .5, .5), []string{"resampling"}},
		{"one axis", scaleWarnings("bilinear", 1, 2), []string{"resampling"}},
		{"identity bilinear", scaleWarnings("bilinear", 1, 1), nil},
		{"identity rotsprite", scaleWarnings("rotsprite", 1, 1), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var codes []string
			for _, w := range tt.warnings {
				require.NotEmpty(t, w.Message)
				codes = append(codes, w.Code)
			}
			require.Equal(t, tt.codes, codes)
		})
	}
}

func TestWarningJSONCompatibility(t *testing.T) {
	for _, output := range []any{
		FlattenLayersOutput{Success: true},
		QuantizePaletteOutput{Success: true},
		ScaleSpriteOutput{Success: true, NewWidth: 16, NewHeight: 16},
	} {
		data, err := json.Marshal(output)
		require.NoError(t, err)
		require.NotContains(t, string(data), `"warnings"`)
	}
	data, err := json.Marshal(FlattenLayersOutput{Success: true, Warnings: flattenWarnings()})
	require.NoError(t, err)
	// An existing permissive JSON client can still consume its original fields.
	var legacy struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(data, &legacy))
	require.True(t, legacy.Success)
	var current FlattenLayersOutput
	require.NoError(t, json.Unmarshal([]byte(`{"success":true}`), &current))
	require.Empty(t, current.Warnings)
}

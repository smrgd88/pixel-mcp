package tools

// ToolWarning describes a potentially destructive effect of a completed operation.
// Codes are stable machine-readable identifiers; messages are for display only.
// Warnings do not indicate failure, request confirmation, or provide undo.
type ToolWarning struct {
	Code    string `json:"code" jsonschema:"Stable warning code"`
	Message string `json:"message" jsonschema:"Human-readable description of the operation's potential effect"`
}

func quantizationWarnings(convertToIndexed, dither bool) []ToolWarning {
	warnings := []ToolWarning{{
		Code:    "palette_quantization",
		Message: "Quantization replaces the palette and may discard color information. Keep a backup to preserve the original colors.",
	}}
	if convertToIndexed {
		warnings = append(warnings, ToolWarning{
			Code:    "color_mode_conversion",
			Message: "Indexed color conversion was requested and may change color or transparency representation. Keep a backup to preserve the original representation.",
		})
	}
	if dither {
		// ReplaceWithImage flattens the sprite before replacing the first cel.
		warnings = append(warnings, flattenWarnings()...)
	}
	return warnings
}

func flattenWarnings() []ToolWarning {
	return []ToolWarning{{
		Code:    "layer_flattening",
		Message: "Flattening merges layers into one and may discard editable layer structure. Keep a backup to preserve separate layers.",
	}}
}

func scaleWarnings(algorithm string, scaleX, scaleY float64) []ToolWarning {
	if (algorithm != "bilinear" && algorithm != "rotsprite") || (scaleX == 1 && scaleY == 1) {
		return nil
	}
	return []ToolWarning{{
		Code:    "resampling",
		Message: "Scaling with a non-nearest algorithm may change pixel colors or edge patterns. Keep a backup to preserve the original pixel art.",
	}}
}

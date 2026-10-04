package tools

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestStructureInputValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   GetSpriteStructureInput
	}{
		{"missing path", GetSpriteStructureInput{}},
		{"negative offset", GetSpriteStructureInput{SpritePath: "x", LayerOffset: -1}},
		{"oversize", GetSpriteStructureInput{SpritePath: "x", PageSize: 101}},
		{"negative page", GetSpriteStructureInput{SpritePath: "x", PageSize: -1}},
		{"reversed", GetSpriteStructureInput{SpritePath: "x", FrameStart: 2, FrameEnd: 1}},
		{"frame span", GetSpriteStructureInput{SpritePath: "x", FrameEnd: 101}},
		{"negative frame", GetSpriteStructureInput{SpritePath: "x", FrameStart: -1}},
		{"zero sibling", GetSpriteStructureInput{SpritePath: "x", LayerID: "2/0"}},
		{"leading zero", GetSpriteStructureInput{SpritePath: "x", LayerID: "02"}},
		{"name path", GetSpriteStructureInput{SpritePath: "x", LayerID: "group/paint"}},
	} {
		t.Run(tc.name, func(t *testing.T) { _, err := validateStructureInput(tc.in); require.Error(t, err) })
	}
	in, err := validateStructureInput(GetSpriteStructureInput{SpritePath: "x", LayerID: "2/1"})
	require.NoError(t, err)
	require.Equal(t, 50, in.PageSize)
	require.Equal(t, 1, in.FrameStart)
	_, err = validateStructureInput(GetSpriteStructureInput{SpritePath: "x", PageSize: 100, FrameStart: 2, FrameEnd: 101})
	require.NoError(t, err)
	require.False(t, historyEdit("get_sprite_structure", in))
}

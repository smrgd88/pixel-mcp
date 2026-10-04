package tools

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestCelPropertiesValidation(t *testing.T) {
	zero := 0
	valid := SetCelPropertiesInput{SpritePath: "sprite.aseprite", LayerID: "2/1", FrameNumber: 1, ExpectedRevision: strings.Repeat("a", 64), X: &zero}
	require.NoError(t, validateCelProperties(valid))
	for _, v := range []int{-32768, 0, 32767} {
		in := valid
		in.Y = &v
		require.NoError(t, validateCelProperties(in))
	}
	for _, v := range []int{0, 255} {
		in := valid
		in.Opacity = &v
		require.NoError(t, validateCelProperties(in))
	}
	for _, modify := range []func(*SetCelPropertiesInput){
		func(i *SetCelPropertiesInput) { i.X = nil },
		func(i *SetCelPropertiesInput) { i.SpritePath = "sprite.png" },
		func(i *SetCelPropertiesInput) { i.LayerID = "01" },
		func(i *SetCelPropertiesInput) { i.LayerID = "group/paint" },
		func(i *SetCelPropertiesInput) { i.ExpectedRevision = "" },
		func(i *SetCelPropertiesInput) { i.ExpectedRevision = strings.Repeat("z", 64) },
		func(i *SetCelPropertiesInput) { i.FrameNumber = 0 },
		func(i *SetCelPropertiesInput) { i.FrameNumber = 65536 },
		func(i *SetCelPropertiesInput) { v := -32769; i.X = &v },
		func(i *SetCelPropertiesInput) { v := 32768; i.Y = &v },
		func(i *SetCelPropertiesInput) { v := -1; i.Opacity = &v },
		func(i *SetCelPropertiesInput) { v := 256; i.Opacity = &v },
	} {
		in := valid
		modify(&in)
		require.Error(t, validateCelProperties(in))
	}
}

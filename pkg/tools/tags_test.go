package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTagPropertiesValidation(t *testing.T) {
	zero := 0
	valid := SetTagPropertiesInput{SpritePath: "sprite.aseprite", TagID: 1, ExpectedRevision: strings.Repeat("a", 64), Repeats: &zero}
	require.NoError(t, validateTagProperties(valid))
	for _, v := range []int{0, 1, 65535} {
		in := valid
		in.Repeats = &v
		require.NoError(t, validateTagProperties(in))
	}
	for _, v := range []string{"forward", "reverse", "pingpong", "pingpong_reverse"} {
		in := valid
		in.Direction = &v
		require.NoError(t, validateTagProperties(in))
	}
	for _, v := range []string{"", " \t\n", "\x00", "a\x00b", string([]byte{255}), strings.Repeat("x", 65536)} {
		in := valid
		in.Name = &v
		require.Error(t, validateTagProperties(in))
	}
	for _, modify := range []func(*SetTagPropertiesInput){
		func(i *SetTagPropertiesInput) { i.Repeats = nil },
		func(i *SetTagPropertiesInput) { i.SpritePath = "a.png" },
		func(i *SetTagPropertiesInput) { i.TagID = 0 },
		func(i *SetTagPropertiesInput) { i.TagID = 65536 },
		func(i *SetTagPropertiesInput) { i.ExpectedRevision = "" },
		func(i *SetTagPropertiesInput) { i.ExpectedRevision = strings.Repeat("A", 64) },
		func(i *SetTagPropertiesInput) { v := 0; i.FromFrame = &v },
		func(i *SetTagPropertiesInput) { v := -1; i.ToFrame = &v },
		func(i *SetTagPropertiesInput) { v := 65536; i.ToFrame = &v },
		func(i *SetTagPropertiesInput) { a, b := 2, 1; i.FromFrame = &a; i.ToFrame = &b },
		func(i *SetTagPropertiesInput) { v := -1; i.Repeats = &v },
		func(i *SetTagPropertiesInput) { v := 65536; i.Repeats = &v },
		func(i *SetTagPropertiesInput) { v := ""; i.Direction = &v },
		func(i *SetTagPropertiesInput) { v := "FORWARD"; i.Direction = &v },
	} {
		in := valid
		modify(&in)
		require.Error(t, validateTagProperties(in))
	}
	name := strings.Repeat("a", 65535)
	valid.Name = &name
	require.NoError(t, validateTagProperties(valid))
	require.False(t, historyEdit("get_sprite_tags", GetSpriteTagsInput{}))
	require.True(t, historyEdit("set_tag_properties", valid))
}

package tools

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestUnlinkCelValidation(t *testing.T) {
	valid := UnlinkCelInput{SpritePath: "sprite.ASE", LayerID: "2/1", FrameNumber: 1, ExpectedRevision: strings.Repeat("a", 64)}
	require.NoError(t, validateUnlinkCel(valid))
	for _, edit := range []func(*UnlinkCelInput){
		func(i *UnlinkCelInput) { i.SpritePath = "sprite.png" }, func(i *UnlinkCelInput) { i.LayerID = "01" }, func(i *UnlinkCelInput) { i.LayerID = "2//1" },
		func(i *UnlinkCelInput) { i.LayerID = "1\";error('injected')" }, func(i *UnlinkCelInput) { i.FrameNumber = 0 }, func(i *UnlinkCelInput) { i.FrameNumber = 65536 },
		func(i *UnlinkCelInput) { i.ExpectedRevision = "" }, func(i *UnlinkCelInput) { i.ExpectedRevision = strings.Repeat("A", 64) },
	} {
		in := valid
		edit(&in)
		require.Error(t, validateUnlinkCel(in))
	}
}

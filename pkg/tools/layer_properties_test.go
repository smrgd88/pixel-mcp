package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestLayerPropertiesValidation(t *testing.T) {
	zero := 0
	valid := SetLayerPropertiesInput{SpritePath: "sprite.ASE", ExpectedRevision: strings.Repeat("a", 64), LayerID: "2/1", Opacity: &zero}
	require.NoError(t, validateSetLayerProperties(valid))
	empty := ""
	f := false
	v := valid
	v.Name = &empty
	v.Visible = &f
	v.Editable = &f
	require.NoError(t, validateSetLayerProperties(v))
	for _, mode := range strings.Fields(aseprite.LayerBlendModes) {
		v := valid
		v.BlendMode = &mode
		require.NoError(t, validateSetLayerProperties(v))
	}
	for _, modify := range []func(*SetLayerPropertiesInput){
		func(v *SetLayerPropertiesInput) { v.Opacity = nil },
		func(v *SetLayerPropertiesInput) { v.SpritePath = "sprite.png" },
		func(v *SetLayerPropertiesInput) { v.ExpectedRevision = "" },
		func(v *SetLayerPropertiesInput) { v.ExpectedRevision = strings.Repeat("A", 64) },
		func(v *SetLayerPropertiesInput) { v.LayerID = "01/1" },
		func(v *SetLayerPropertiesInput) { v.LayerID = "same" },
		func(v *SetLayerPropertiesInput) { x := -1; v.Opacity = &x },
		func(v *SetLayerPropertiesInput) { x := 256; v.Opacity = &x },
		func(v *SetLayerPropertiesInput) { x := "src"; v.BlendMode = &x },
		func(v *SetLayerPropertiesInput) { x := "NORMAL"; v.BlendMode = &x },
		func(v *SetLayerPropertiesInput) { x := ""; v.BlendMode = &x },
		func(v *SetLayerPropertiesInput) { x := "a\x00b"; v.Name = &x },
		func(v *SetLayerPropertiesInput) { x := strings.Repeat("a", 65536); v.Name = &x },
	} {
		v := valid
		modify(&v)
		require.Error(t, validateSetLayerProperties(v))
	}
	for _, parent := range []string{"", "1", "2/1"} {
		require.NoError(t, validateLayerDestination(valid.SpritePath, valid.ExpectedRevision, parent, 1))
	}
	for _, parent := range []string{"0", "01", "1/", "same", "/1"} {
		require.Error(t, validateLayerDestination(valid.SpritePath, valid.ExpectedRevision, parent, 1))
	}
	for _, index := range []int{-1, 0, 65536} {
		require.Error(t, validateLayerDestination(valid.SpritePath, valid.ExpectedRevision, "", index))
	}
}

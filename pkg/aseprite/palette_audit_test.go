package aseprite

import (
	"image"
	"image/color"
	"testing"

	"github.com/lucasb-eyer/go-colorful"
	"github.com/stretchr/testify/require"
)

func TestAuditKmeansFirstAssignmentMustUpdateMean(t *testing.T) {
	red := colorful.Color{R: 1}
	blue := colorful.Color{B: 1}
	c := kmeansClustering([]colorful.Color{red, red, blue}, 1, 100)
	require.Len(t, c, 1)
	rl, ra, rb := red.Lab()
	bl, ba, bb := blue.Lab()
	l, a, b := c[0].Lab()
	require.InDelta(t, (2*rl+bl)/3, l, 1e-8)
	require.InDelta(t, (2*ra+ba)/3, a, 1e-8)
	require.InDelta(t, (2*rb+bb)/3, b, 1e-8)
}

func TestAuditExtractPaletteRareColorDeterministic(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			im.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	im.SetRGBA(2, 3, color.RGBA{G: 255, A: 255})
	var baseline []PaletteColor
	for n := 0; n < 10; n++ {
		p, err := ExtractPalette(im, 5)
		require.NoError(t, err)
		require.Len(t, p, 5)
		usage := map[string]float64{}
		for _, c := range p {
			usage[c.Color] += c.UsagePercent
		}
		require.InDelta(t, 98.4375, usage["#FF0000"], 1e-8)
		require.InDelta(t, 1.5625, usage["#00FF00"], 1e-8)
		if n == 0 {
			baseline = p
		} else {
			require.Equal(t, baseline, p)
		}
	}
}

package aseprite

import (
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestApplyAutoShading_GeneratedColorsFollowRegionDiscoveryOrder(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	brown := color.RGBA{R: 122, G: 62, B: 29, A: 255}
	green := color.RGBA{R: 46, G: 139, B: 87, A: 255}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.SetRGBA(x, y, brown)
		}
		for x := 4; x < 8; x++ {
			img.SetRGBA(x, y, green)
		}
	}

	var baseline []string
	for run := 0; run < 25; run++ {
		_, generated, _, err := ApplyAutoShading(img, "top_left", 0.5, "smooth", true)
		if err != nil {
			t.Fatalf("ApplyAutoShading() run %d error = %v", run, err)
		}
		if len(generated) != 6 {
			t.Fatalf("generated color count = %d, want 6", len(generated))
		}
		if generated[1] != "#7A3E1D" || generated[4] != "#2E8B57" {
			t.Fatalf("base colors are not in discovery order: %v", generated)
		}
		if run == 0 {
			baseline = append([]string(nil), generated...)
		} else if !reflect.DeepEqual(generated, baseline) {
			t.Fatalf("generated colors changed on run %d: got %v, want %v", run, generated, baseline)
		}
	}
}

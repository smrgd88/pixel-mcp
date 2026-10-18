package tools

import (
	"context"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestExportBudgetBounds(t *testing.T) {
	for _, tc := range []struct {
		w, h, n, b, s, i int
		layout           string
		ok               bool
	}{
		{32767, 1, 1, 0, 0, 0, "horizontal", true},
		{32767, 1, 1, 1, 0, 0, "vertical", false},
		{1, 32767, 1, 0, 0, 1, "horizontal", false},
		{8192, 8192, 1, 0, 0, 0, "packed", true},
		{8192, 8192, 2, 0, 0, 0, "packed", false},
		{1, 1, 65535, 0, 0, 0, "horizontal", false},
		{1, 1, 65536, 0, 0, 0, "packed", false},
		{1, 1, 1, math.MaxInt, 0, 0, "packed", false},
		{math.MaxInt, 1, 1, 0, 0, 0, "packed", false},
		{1, 1, math.MaxInt, 0, 0, 0, "packed", false},
		{128, 128, 65, 0, 0, 0, "packed", true},
	} {
		err := validateExportDimensions(exportPlan{Width: tc.w, Height: tc.h, First: 1, Last: tc.n}, tc.layout, tc.b, tc.s, tc.i, false)
		if tc.ok {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := allocateSheet(ctx, exportPlan{Width: 1, Height: 1, First: 1, Last: 1}, []aseprite.ExportSample{{W: 1, H: 1, Same: 1}}, "packed", 0, 0, 0, false)
	require.ErrorIs(t, err, context.Canceled)
}

// A deliberately slow occupancy-grid oracle checks the optimized skip logic
// against exhaustive pixel placement. Small deterministic cases avoid OOMs.
func TestExportBudgetPackingProperties(t *testing.T) {
	rng := rand.New(rand.NewSource(391))
	for trial := 0; trial < 500; trial++ {
		n := rng.Intn(10) + 1
		samples := make([]aseprite.ExportSample, n)
		for i := range samples {
			samples[i] = aseprite.ExportSample{W: rng.Intn(9) + 1, H: rng.Intn(9) + 1, Same: i + 1}
		}
		border, shape, inner := rng.Intn(4), rng.Intn(5), rng.Intn(3)
		extrude := rng.Intn(2) == 0
		a, err := allocateSheet(context.Background(), exportPlan{Width: 10, Height: 10, First: 1, Last: n}, samples, "packed", border, shape, inner, extrude)
		require.NoError(t, err)
		rects := append([]exportRect(nil), a.Frames...)
		for i := range rects {
			rects[i].X = 0
			rects[i].Y = 0
			if extrude {
				rects[i].W += 2
				rects[i].H += 2
			}
		}
		want := pixelPackingOracle(rects, border, shape)
		if extrude {
			for i := range want {
				want[i].X++
				want[i].Y++
				want[i].W -= 2
				want[i].H -= 2
			}
		}
		require.Equal(t, want, a.Frames, "trial %d", trial)
	}
}

func pixelPackingOracle(rects []exportRect, border, shape int) []exportRect {
	order := make([]int, len(rects))
	w0, h0, area := 1, 1, 0
	for i, r := range rects {
		order[i] = i
		w0 = max(w0, r.W)
		h0 = max(h0, r.H)
		area += r.W * r.H
	}
	// Stable selection independent of production sorting.
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			a, b := rects[order[i]], rects[order[j]]
			if b.W*b.H > a.W*a.H || (b.W*b.H == a.W*a.H && order[j] < order[i]) {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	w, h := w0, h0
	for k := 0; ; k++ {
		if w*h >= area {
			cells := make([]bool, w*h)
			fit := true
			for _, i := range order {
				r := rects[i]
				placed := false
				for y := 0; y+r.H <= h && !placed; y++ {
					for x := 0; x+r.W <= w; x++ {
						pw, ph := r.W, r.H
						if x+r.W < w {
							pw += shape
						}
						if y+r.H < h {
							ph += shape
						}
						if x+pw > w || y+ph > h {
							continue
						}
						free := true
						for yy := y; yy < y+ph; yy++ {
							for xx := x; xx < x+pw; xx++ {
								if cells[yy*w+xx] {
									free = false
								}
							}
						}
						if !free {
							continue
						}
						for yy := y; yy < y+ph; yy++ {
							for xx := x; xx < x+pw; xx++ {
								cells[yy*w+xx] = true
							}
						}
						rects[i].X = x + border
						rects[i].Y = y + border
						placed = true
						break
					}
				}
				if !placed {
					fit = false
					break
				}
			}
			if fit {
				return rects
			}
		}
		if k%2 == 0 {
			w += w0
		} else {
			h += h0
		}
	}
}

func TestExportBudgetPlanningWorkLimit(t *testing.T) {
	// Geometry only: no oversized canvas or native bitmap is allocated.
	samples := make([]aseprite.ExportSample, 2000)
	for i := range samples {
		samples[i] = aseprite.ExportSample{W: 1, H: 1, Same: i + 1}
	}
	_, err := allocateSheet(context.Background(), exportPlan{Width: 1, Height: 1, First: 1, Last: len(samples)}, samples, "packed", 0, 0, 0, false)
	require.ErrorContains(t, err, "planning budget")
	// The content fits the area budget but its surrounding border does not.
	require.Error(t, validateExportDimensions(exportPlan{Width: 8191, Height: 8191, First: 1, Last: 1}, "packed", 1, 0, 0, false))
}

func TestExportBudgetRuntimePackingPolicy(t *testing.T) {
	for version, want := range map[string]bool{"1.3.17.2": false, "1.3.18": false, "1.3.18.1": false, "1.3.18.2-arm64": true, "1.3.18.3-dev": true, "invalid": false} {
		require.Equal(t, want, aseprite.HasStableSheetPacking(version), version)
	}
	p := exportPlan{Width: 16, Height: 16, First: 1, Last: 24}
	samples := make([]aseprite.ExportSample, 24)
	for i := range samples {
		samples[i] = aseprite.ExportSample{W: 2, H: 6, Same: i + 1}
	}
	a, err := allocateSheetMode(context.Background(), p, samples, "packed", 1, 2, 0, true, true)
	require.NoError(t, err)
	require.True(t, a.Conservative)
	require.Equal(t, 24*4+23*2+2, a.Width)
	require.Equal(t, 24*8+23*2+2, a.Height)
}

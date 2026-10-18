// Packing rules adapted from LAF gfx/packing_rects.cpp (MIT):
// Copyright (C) 2019-2023 Igara Studio S.A.
// Copyright (C) 2001-2014 David Capello
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies
// of the Software, and to permit persons to whom the Software is furnished to
// do so, subject to the following conditions:
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package tools

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

const exportMaxSide = 32767
const exportMaxPixels = 64 * 1024 * 1024

type exportRect struct{ X, Y, W, H int }
type sheetAllocation struct {
	Width, Height int
	Conservative  bool
	Frames        []exportRect
	Samples       []aseprite.ExportSample
}

func exportBudgetError() error {
	return diagnostics.Errorf("invalid_arguments", "spritesheet exceeds dimension or planning budget")
}

func measureSheet(ctx context.Context, c *aseprite.Client, g *aseprite.LuaGenerator, path string, p exportPlan, o aseprite.ExportSelection, layout string, border, shape, inner int, trim, extrude bool) (sheetAllocation, error) {
	var allocation sheetAllocation
	err := aseprite.WithSpriteAccess(ctx, path, false, func(ctx context.Context) error {
		if err := verifyExportRevision(ctx, path, p.Revision); err != nil {
			return err
		}
		var err error
		allocation, err = measureBoundSheet(ctx, c, g, path, p, o, layout, border, shape, inner, trim, extrude)
		return err
	})
	return allocation, err
}

func measureBoundSheet(ctx context.Context, c *aseprite.Client, g *aseprite.LuaGenerator, path string, p exportPlan, o aseprite.ExportSelection, layout string, border, shape, inner int, trim, extrude bool) (sheetAllocation, error) {
	if err := validateExportDimensions(p, "", 0, 0, 0, false); err != nil {
		return sheetAllocation{}, err
	}
	raw, err := c.ExecuteLua(ctx, g.MeasureExportSheet(o, trim, layout == "packed"), path)
	if err != nil {
		return sheetAllocation{}, err
	}
	var measured struct {
		Samples     []aseprite.ExportSample `json:"samples"`
		BudgetError string                  `json:"budget_error"`
		Version     string                  `json:"version"`
	}
	if err = parseJSON(raw, &measured); err != nil {
		return sheetAllocation{}, err
	}
	if measured.BudgetError != "" {
		return sheetAllocation{}, diagnostics.Errorf("invalid_arguments", "%s", measured.BudgetError)
	}
	if len(measured.Samples) != p.Last-p.First+1 {
		return sheetAllocation{}, fmt.Errorf("export measurement frame count mismatch")
	}
	if err = verifyExportRevision(ctx, path, p.Revision); err != nil {
		return sheetAllocation{}, err
	}
	return allocateSheetMode(ctx, p, measured.Samples, layout, border, shape, inner, extrude, layout == "packed" && !aseprite.HasStableSheetPacking(measured.Version))
}

// allocateSheet follows Aseprite DocExporter and LAF PackingRects, rather than
// imposing a hypothetical grid. Rows/columns without constraints are strips.
// Packed uses stable descending area and the first free pixel in scan order,
// never rotation. See EXPORT_OPTIONS for pinned source and differential tests.
func allocateSheet(ctx context.Context, p exportPlan, samples []aseprite.ExportSample, layout string, border, shape, inner int, extrude bool) (sheetAllocation, error) {
	return allocateSheetMode(ctx, p, samples, layout, border, shape, inner, extrude, false)
}

func allocateSheetMode(ctx context.Context, p exportPlan, samples []aseprite.ExportSample, layout string, border, shape, inner int, extrude, conservative bool) (sheetAllocation, error) {
	result := sheetAllocation{Samples: samples, Conservative: conservative}
	if err := validateExportDimensions(p, "", 0, 0, 0, false); err != nil {
		return result, err
	}
	if len(samples) != p.Last-p.First+1 || border < 0 || border > 100 || shape < 0 || shape > 100 || inner < 0 || inner > 100 {
		return result, exportBudgetError()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	rim := 0
	if extrude {
		rim = 2
	}
	rects := make([]exportRect, len(samples))
	order := make([]int, 0, len(samples))
	var area int64
	for i, s := range samples {
		if s.X < 0 || s.Y < 0 || s.W < 1 || s.H < 1 || s.W > p.Width || s.H > p.Height || s.X > p.Width-s.W || s.Y > p.Height-s.H || s.Same < 1 || s.Same > i+1 {
			return result, exportBudgetError()
		}
		if s.Same != i+1 {
			a := samples[s.Same-1]
			if a.Same != s.Same || a.W != s.W || a.H != s.H {
				return result, exportBudgetError()
			}
		}
		r := exportRect{W: s.W + 2*inner + rim, H: s.H + 2*inner + rim}
		if r.W+2*border > exportMaxSide || r.H+2*border > exportMaxSide {
			return result, exportBudgetError()
		}
		rects[i] = r
		if layout != "packed" || s.Same == i+1 {
			order = append(order, i)
			area += int64(r.W) * int64(r.H)
		}
	}
	if area > exportMaxPixels {
		return result, exportBudgetError()
	}
	switch layout {
	case "horizontal", "rows", "vertical", "columns":
		x, y := border, border
		for i := range rects {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			rects[i].X = x
			rects[i].Y = y
			if layout == "horizontal" || layout == "rows" {
				x += rects[i].W + shape
			} else {
				y += rects[i].H + shape
			}
			if x > exportMaxSide+shape || y > exportMaxSide+shape {
				return result, exportBudgetError()
			}
		}
	case "packed":
		if conservative {
			// Pre-1.3.18.2 native std::sort may permute equal-area samples
			// differently across standard libraries. Retain the proven strip
			// envelope for these runtimes, refined by trim and duplicates.
			// Never run an unbounded native bitmap as a size probe.
			if err := ctx.Err(); err != nil {
				return result, err
			}
			var w, h int64
			for _, i := range order {
				w += int64(rects[i].W)
				h += int64(rects[i].H)
			}
			padding := int64(len(order)-1)*int64(shape) + 2*int64(border)
			w += padding
			h += padding
			if w > exportMaxSide || h > exportMaxSide || w*h > exportMaxPixels {
				return result, exportBudgetError()
			}
			result.Width = int(w)
			result.Height = int(h)
			break
		}
		if err := packSheet(ctx, rects, order, border, shape, area); err != nil {
			return result, err
		}
		for i, s := range samples {
			if s.Same != i+1 {
				rects[i] = rects[s.Same-1]
			}
		}
	default:
		return result, exportBudgetError()
	}
	for _, r := range rects {
		result.Width = max(result.Width, r.X+r.W+border)
		result.Height = max(result.Height, r.Y+r.H+border)
	}
	if result.Width > exportMaxSide || result.Height > exportMaxSide || int64(result.Width)*int64(result.Height) > exportMaxPixels {
		return result, exportBudgetError()
	}
	// JSON includes inner padding but excludes the one-pixel extrusion rim.
	if extrude {
		for i := range rects {
			rects[i].X++
			rects[i].Y++
			rects[i].W -= 2
			rects[i].H -= 2
		}
	}
	result.Frames = rects
	return result, nil
}

func packSheet(ctx context.Context, rects []exportRect, order []int, border, shape int, area int64) error {
	w0, h0 := 1, 1
	for _, i := range order {
		w0 = max(w0, rects[i].W)
		h0 = max(h0, rects[i].H)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := rects[order[i]], rects[order[j]]
		return int64(a.W)*int64(a.H) > int64(b.W)*int64(b.H)
	})
	w, h := w0, h0
	// Count geometry operations, not bitmap pixels. This bounds adversarial
	// fragmented packing even without a request deadline; cancellation is checked
	// during candidate placement, not just between complete packing attempts.
	work := 0
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Native bestFit uses signed int area. Never enter its overflow region.
		if int64(w)*int64(h) > int64(1<<31-1) || w > 2*exportMaxSide || h > 2*exportMaxSide {
			return exportBudgetError()
		}
		if int64(w)*int64(h) >= area {
			occupied := make([]exportRect, 0, len(order))
			fit := true
			for _, i := range order {
				r := rects[i]
				placed := false
				for y := 0; y <= h-r.H && !placed; {
					ph := r.H
					if y != h-r.H {
						ph += shape
					}
					for x := 0; x <= w-r.W; {
						work++
						if work > 8000000 {
							return exportBudgetError()
						}
						if work%1024 == 0 {
							if err := ctx.Err(); err != nil {
								return err
							}
						}
						pw := r.W
						if x != w-r.W {
							pw += shape
						}
						if x+pw > w || y+ph > h {
							x++
							continue
						}
						next := x
						for _, q := range occupied {
							work++
							if work > 8000000 {
								return exportBudgetError()
							}
							if x < q.X+q.W && q.X < x+pw && y < q.Y+q.H && q.Y < y+ph {
								next = max(next, q.X+q.W)
							}
						}
						if next != x {
							x = next
							continue
						}
						occupied = append(occupied, exportRect{x, y, pw, ph})
						rects[i].X = x + border
						rects[i].Y = y + border
						placed = true
						break
					}
					if !placed {
						nextY := h - r.H
						for _, q := range occupied {
							if q.Y+q.H > y {
								nextY = min(nextY, q.Y+q.H)
							}
						}
						if nextY <= y {
							break
						}
						y = nextY
					}
				}
				if !placed {
					fit = false
					break
				}
			}
			if fit {
				return nil
			}
		}
		if attempt%2 == 0 {
			w += w0
		} else {
			h += h0
		}
	}
}

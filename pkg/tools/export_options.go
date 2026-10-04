package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/willibrandon/pixel-mcp/internal/diagnostics"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

type exportPlan struct {
	First     int   `json:"first"`
	Last      int   `json:"last"`
	Count     int   `json:"count"`
	Width     int   `json:"width"`
	Height    int   `json:"height"`
	Durations []int `json:"durations"`
	Revision  string
}

func validateExportSelection(tag *string, layer, revision string, start, end *int, hidden bool, frame int) (aseprite.ExportSelection, error) {
	o := aseprite.ExportSelection{LayerID: layer, IncludeHidden: hidden, Selected: tag != nil || layer != "" || start != nil || end != nil}
	if tag != nil {
		if *tag == "" {
			return o, diagnostics.Errorf("invalid_arguments", "tag must not be empty")
		}
		o.Tag = *tag
		if start != nil || end != nil || frame > 0 {
			return o, diagnostics.Errorf("invalid_arguments", "tag, frame range and frame_number are mutually exclusive")
		}
	}
	if frame > 0 && (start != nil || end != nil) {
		return o, diagnostics.Errorf("invalid_arguments", "frame_number and frame range are mutually exclusive")
	}
	for _, n := range []*int{start, end} {
		if n != nil && (*n < 1 || *n > 65535) {
			return o, diagnostics.Errorf("invalid_arguments", "frame endpoints must be between 1 and 65535")
		}
	}
	if start != nil {
		o.First = *start
	}
	if end != nil {
		o.Last = *end
	}
	if o.First > 0 && o.Last > 0 && o.First > o.Last {
		return o, diagnostics.Errorf("invalid_arguments", "frame range must be ordered")
	}
	if frame > 0 {
		o.First = frame
		o.Last = frame
	}
	if layer != "" && (!structureLayerID.MatchString(layer) || revision == "") {
		return o, diagnostics.Errorf("invalid_arguments", "layer_id requires a structural ID and expected_revision")
	}
	if revision != "" && !spriteRevisionPattern.MatchString(revision) {
		return o, diagnostics.Errorf("invalid_arguments", "invalid expected_revision")
	}
	if hidden && layer == "" {
		return o, diagnostics.Errorf("invalid_arguments", "include_hidden requires layer_id")
	}
	return o, nil
}

func planExport(ctx context.Context, c *aseprite.Client, g *aseprite.LuaGenerator, path, expected string, o aseprite.ExportSelection) (exportPlan, error) {
	var p exportPlan
	err := aseprite.WithSpriteAccess(ctx, path, false, func(ctx context.Context) error {
		before, err := aseprite.SpriteRevision(ctx, path)
		if err != nil {
			return err
		}
		if expected != "" && before != expected {
			return diagnostics.Errorf("file_changed", "stale sprite revision; query structure again")
		}
		raw, err := c.ExecuteLua(ctx, g.PlanExport(o), path)
		if err != nil {
			return err
		}
		if err = parseJSON(raw, &p); err != nil {
			return err
		}
		p.Revision = before
		return verifyExportRevision(ctx, path, before)
	})
	if err != nil {
		return p, err
	}
	if p.Count < 1 || p.Count > 65535 || p.First < 1 || p.Last < p.First || p.Last > p.Count || len(p.Durations) != p.Last-p.First+1 {
		return p, fmt.Errorf("invalid export plan")
	}
	return p, nil
}

func verifyExportRevision(ctx context.Context, path, expected string) error {
	actual, err := aseprite.SpriteRevision(ctx, path)
	if err != nil {
		return err
	}
	if actual != expected {
		return diagnostics.Errorf("file_changed", "source changed during export; retry with a fresh revision")
	}
	return nil
}

// Conservative allocation guards run before native rendering/packing. A packed
// sheet must also fit the worst-case strip; trim cannot bypass the memory bound.
func validateExportDimensions(p exportPlan, layout string, border, shape, inner int, extrude bool) error {
	const maxSide int64 = 32767
	const maxPixels int64 = 64 * 1024 * 1024
	w, h, n := int64(p.Width), int64(p.Height), int64(p.Last-p.First+1)
	if n < 1 || n > 65535 || w < 1 || h < 1 || w > maxSide || h > maxSide || w*h > maxPixels {
		return diagnostics.Errorf("invalid_arguments", "export canvas exceeds dimension budget")
	}
	if layout == "" {
		return nil
	}
	rim := int64(0)
	if extrude {
		rim = 2
	}
	cw, ch := w+2*int64(inner)+rim, h+2*int64(inner)+rim
	spanW, spanH := n*cw+(n-1)*int64(shape)+2*int64(border), n*ch+(n-1)*int64(shape)+2*int64(border)
	if cw*ch*n > maxPixels || ((layout != "vertical") && spanW > maxSide) || ((layout != "horizontal") && spanH > maxSide) {
		return diagnostics.Errorf("invalid_arguments", "spritesheet exceeds conservative dimension budget")
	}
	// A native grid/packing result fits within these conservative strip extents.
	// Use the enclosing rectangle for non-strip layouts, including unused space.
	area := spanW * spanH
	if layout == "horizontal" {
		area = spanW * (ch + 2*int64(border))
	}
	if layout == "vertical" {
		area = (cw + 2*int64(border)) * spanH
	}
	if area > maxPixels {
		return diagnostics.Errorf("invalid_arguments", "spritesheet exceeds pixel budget")
	}
	return nil
}

func checkExportOverwrite(paths []string, overwrite *bool) error {
	if overwrite == nil || *overwrite {
		return nil
	}
	for _, p := range paths {
		_, err := os.Lstat(p)
		if err == nil {
			return diagnostics.Errorf("invalid_arguments", "overwrite=false and an output already exists")
		}
		if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// Recheck the no-overwrite precondition after output snapshots/backups exist.
// Otherwise an external file appearing between the handler's initial existence
// check and staging would become the accepted baseline and be overwritten.
func withExportOutputFiles(ctx context.Context, paths []string, overwrite *bool, fn func([]string) error) error {
	return aseprite.WithOutputFiles(ctx, paths, func(staged []string) error {
		if err := checkExportOverwrite(paths, overwrite); err != nil {
			return err
		}
		return fn(staged)
	})
}

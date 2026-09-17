# Functional behavior repairs

Base: develop `3a20dedb4ae7edce2e0151a210cef2347bf5e9b9`. This work is separate from the operation-warnings branch and does not merge it or change tool names/input schemas.

## Reproductions and fixes

| Area | Failure | Repair and executable evidence |
|---|---|---|
| Indexed pixels | Default Color conversion wrote an out-of-palette index | Resolve an index against the sprite palette; `TestBehavior_IndexedPixelsWithoutPaletteFlag` |
| Binary patterns | Threshold multiplied by matrix area made 12 patterns a solid color | Normalize by maximum matrix value + 1; all binary/Bayer patterns checked for both colors |
| Selection persistence | SelectAll saved literal format markers; ellipse became bounds; subtract/intersect lost state | Persist row runs, read legacy bounds, restore before combination, sample within requested ellipse bounds |
| Clipboard | Positioned source cels copied blank data; a small paste target clipped pixels | Map canvas coordinates to cel coordinates and copy only selected pixels; expand destination without losing existing content |
| Quantization | Alpha parsing rejected transparent colors; palette exceeded target; octree omitted root reduction; RGB result said nil | Parse RGBA, reserve one transparent slot, include reducible root, name mode explicitly; all three algorithms and both result modes tested |
| Frame duplication | Repeated append inserted near source and shifted existing action poses | Snapshot independent images before insertion, create empty frame at the actual requested index, return its real number |

Regression tests are in `pkg/tools/behavior_regression_test.go` and `pkg/aseprite/quantization_test.go`. They call MCP handlers and reopen saved files through real Aseprite; success flags alone are insufficient.

Selection state now uses the private pixel-mcp/selection extension-property namespace. User sprite.data remains byte-for-byte unchanged; legacy bounds/runs can be read without rewriting them, and an explicit version marker prevents stale legacy masks from returning after deselect. Copy has no source selector and continues to use first layer/frame. Clipboard storage is the hidden same-sprite layer, not an OS clipboard.

The quantizer's existing dither workflow still flattens/replaces artwork; these repairs do not turn it into a layer-preserving or arbitrary multi-frame remapper. Callers should work on a native copy. A separate warnings effort remains outside this branch.

## Validation

macOS arm64, Aseprite 1.3.18.2, Go 1.25.0. Full `go test -json -tags=integration ./...`: 861 test/subtest entries passed with no failed or skipped tests; packages with no tests are reported separately. `go vet ./...` and whitespace checks passed. The companion plugin runs a 50-tool/102-scenario behavior sweep and complete artwork recipes against this implementation.

An earlier full run under simultaneous Aseprite workloads hit the existing 30-second downsampling timeout. The final suite ran separately and passed; the timeout limit was not weakened. Non-macOS execution and every possible option/input combination are not claimed.

## Self-review follow-up

The prior manual frame clone omitted grouped cels and cel data/zIndex. Duplication now uses Aseprite native cloning and moves the resulting cels to the requested frame, retaining nested groups, timeline color, opacity, user data and extension properties. Insertion happens at the final location before temporary cloning, preserving normal tag-boundary behavior.

The previous JSON-to-Lua conversion lost nulls and empty-object types. The selection repair now never rewrites user sprite.data. New regression tests cover nested groups, source/destination ordering, composite zIndex results, namespaced cel properties, tag boundaries, raw metadata and clearing migrated legacy state.

API references used in the repair: [Sprite](https://www.aseprite.org/api/sprite/), [Cel](https://www.aseprite.org/api/cel/), [Properties](https://www.aseprite.org/api/properties/), [JSON](https://www.aseprite.org/api/json/). Actual saved-file tests validate these behaviors on the supported runtime.

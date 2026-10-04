# pixel-mcp

[Capability inventory and roadmap](docs/CAPABILITIES.md) · [Roadmap](docs/ROADMAP.md) · [Changelog](CHANGELOG.md) · [Runtime capability checks](docs/CAPABILITIES.md#실행-환경-사전-검사-gap-10) · [Operation warning contract](docs/WARNINGS.md)

[한국어](README.md) · [Original README backup](README.original.md)

pixel-mcp is a local MCP (Model Context Protocol) server that lets AI clients control Aseprite.

This public fork is developed primarily with OpenAI Codex, and the getting-started guide below follows that workflow. The server uses standard MCP over stdio and can also connect to other compatible clients.

This README describes `develop`. Fork fixes and recovery features are **Unreleased**, not part of the existing `v0.5.0` distribution. Build from source below to use the development features.

## Features

- Create and manage canvases, layers, and frames
- Draw pixels and basic shapes
- Work with palettes, selections, and animations
- Inspect sprite metadata and pixel colors
- Export PNG, GIF, and spritesheets
- Operation warnings, dry-run, snapshot/restore, and opt-in operation history/undo

## MCP tools

The `develop` baseline registers 58 tools through PR #31 (Unreleased). See the [capability inventory](docs/CAPABILITIES.md) for support boundaries and limitations.

### Canvas and layers

`create_canvas`, `add_layer`, `delete_layer`, `flatten_layers`, `get_sprite_info`

Create, remove, and flatten canvases and layers, and inspect sprite metadata.

### Drawing

`draw_pixels`, `draw_line`, `draw_contour`, `draw_rectangle`, `draw_circle`, `fill_area`

Draw pixels, lines, contours, shapes, and fills with optional palette snapping.

### Selection and clipboard

`select_rectangle`, `select_ellipse`, `select_all`, `deselect`, `move_selection`, `cut_selection`, `copy_selection`, `paste_clipboard`

Selection and clipboard state persists across consecutive MCP calls.

### Pixel art and palettes

`analyze_reference`, `draw_with_dither`, `downsample_image`, `quantize_palette`, `get_palette`, `set_palette`, `set_palette_color`, `add_palette_color`, `sort_palette`, `apply_shading`, `apply_auto_shading`, `analyze_palette_harmonies`, `suggest_antialiasing`

Analyze references, apply dithering, downsample images, quantize colors, edit palettes, shade automatically, and suggest antialiasing.

For indexed sprites, `apply_auto_shading` preserves existing palette indices and the transparent index. New shade colors are appended only when capacity allows; otherwise non-exact shades retain their original pixel indices.

### Transform

`flip_sprite`, `rotate_sprite`, `scale_sprite`, `crop_sprite`, `resize_canvas`, `apply_outline`

Transform, resize, crop, and outline sprites.

### Animation

`add_frame`, `delete_frame`, `set_frame_duration`, `create_tag`, `delete_tag`, `duplicate_frame`, `link_cel`, `set_cel_properties`

Manage frames, timing, tags, and Aseprite native linked cels. `link_cel` preserves shared image identity after saving and rejects an already occupied target cel to protect its data.

### Inspection and file operations

`get_sprite_structure`, `get_pixels`, `export_sprite`, `export_spritesheet`, `import_image`, `save_as`

Range export candidate: tag/frame ranges, revision-guarded layer/group selection, trim, sheet extrusion/individual padding, and texture+JSON output protection. See the [options, compatibility and verification contract](docs/EXPORT_OPTIONS.md).

Verify pixels and work with PNG, GIF, JPG, BMP, spritesheets, and Aseprite files.

[Detailed structure inspection](docs/SPRITE_STRUCTURE.md) returns read-only hierarchy and per-frame cel existence, position, size, opacity and native image sharing. Structural IDs are not persistent IDs. [Cel editing](docs/CEL_PROPERTIES.md) requires the returned `revision` to set position/opacity; changing the entire native linked set requires explicit `allow_linked=true`. Other mutation inputs are unchanged. Layer pages and frame windows each have a maximum of 100.

### Recovery and operation history (Unreleased)

`create_snapshot`, `list_snapshots`, `restore_snapshot`, `delete_snapshot`, `list_operation_history`, `undo_last_operation`

Create and restore snapshots of saved files. Restore creates a backup first. Copies expire after 7 days; the store is limited to 100 snapshots / 512 MiB. Optional absolute `snapshot_dir` config overrides `os.UserConfigDir()/pixel-mcp/snapshots`; this is separate from `temp_dir`. See [contract and limits](docs/SNAPSHOTS.md).

Set `"enable_history": true` in the config to record successful in-place edits (default false). Use `list_operation_history` with `sprite_path`, then call `undo_last_operation` with that path and the latest applied `expected_operation_id`. External changes and stale IDs are rejected. Automatic copies share snapshot capacity and expiry; see the [history contract](docs/HISTORY.md).

## Codex quick start

### 1. Requirements and source build

- Go 1.25+ to build from source
- A local Aseprite executable: version 1.3.17.2+ and API 39+. See the [runtime checks](docs/CAPABILITIES.md#실행-환경-사전-검사-gap-10) for validation history.
- Codex CLI installed in the environment that will run the MCP server

```bash
git clone --branch develop https://github.com/smrgd88/pixel-mcp.git
cd pixel-mcp
go build -o bin/pixel-mcp ./cmd/pixel-mcp
```

Replace `/absolute/path/to/pixel-mcp` below with the actual absolute path to the resulting `bin/pixel-mcp`. Register a path where you will keep the executable.

### 2. Create the server configuration

Save this JSON at a location such as `/absolute/path/to/config.json`. The Aseprite and temporary-directory paths below are macOS examples; replace them for your environment. The server does not automatically discover Aseprite.

```json
{
  "aseprite_path": "/Applications/Aseprite.app/Contents/MacOS/aseprite",
  "temp_dir": "/tmp/pixel-mcp",
  "timeout": 30,
  "log_level": "info"
}
```

Configuration files are selected in this order:

1. `--config <path>`
2. `PIXEL_MCP_CONFIG`
3. `~/.config/pixel-mcp/config.json`

### 3. Check the Aseprite runtime

```bash
/absolute/path/to/pixel-mcp --config /absolute/path/to/config.json --health
```

A healthy runtime returns exit code 0 and JSON with `success: true`. This checks Aseprite version/API requirements, not the Codex connection itself.

### 4. Register the MCP server in Codex

Use either these CLI commands or the TOML configuration below.

```bash
codex mcp add pixel-mcp -- /absolute/path/to/pixel-mcp --config /absolute/path/to/config.json
codex mcp list
codex mcp get pixel-mcp
```

For manual configuration, add this entry to `~/.codex/config.toml`, preserving your existing settings:

```toml
[mcp_servers.pixel-mcp]
command = "/absolute/path/to/pixel-mcp"
args = ["--config", "/absolute/path/to/config.json"]
```

The JSON configures pixel-mcp; the TOML tells Codex how to launch it. Open a new Codex CLI session and use `/mcp` to inspect active servers. A listed configuration alone does not verify Aseprite tool execution. See the [official Codex MCP guide](https://developers.openai.com/codex/mcp).

Codex launches the server over stdio. Do not add `--health` to its launch arguments: that mode exits after the check. Use `--config` as above. The Codex execution environment must be able to access the server, config, Aseprite, and artwork paths.

### 5. Request a first drawing

Give Codex actual output paths:

> Use pixel-mcp to create a 32×32 RGB canvas and draw a red circle. Save a new file at `/absolute/path/to/art/demo.aseprite`, export `/absolute/path/to/art/demo.png`, and verify the result.

Before destructive operations such as quantization, check [warnings](docs/WARNINGS.md) and [dry-run](docs/DRY_RUN.md) for supported tools, and create a snapshot when needed. Recovery uses saved files rather than the GUI undo stack.

## Other MCP clients

Clients using a `mcpServers` JSON format can adapt this example in their own configuration location. Do not paste this JSON into Codex's `config.toml`.

```json
{
  "mcpServers": {
    "pixel-mcp": {
      "command": "/absolute/path/to/pixel-mcp",
      "args": ["--config", "/absolute/path/to/config.json"]
    }
  }
}
```

## Contributing

For repository development, see the [Codex contributor instructions](AGENTS.md) and [testing guide](docs/TESTING.md).

## License

[MIT](LICENSE)

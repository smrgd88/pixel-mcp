# pixel-mcp

[한국어](README.md) · [Original README backup](README.original.md)

pixel-mcp is a local MCP (Model Context Protocol) server that lets AI clients control Aseprite.

> This public fork is developed and validated with OpenAI Codex.

## Features

- Create and manage canvases, layers, and frames
- Draw pixels and basic shapes
- Work with palettes, selections, and animations
- Inspect sprite metadata and pixel colors
- Export PNG, GIF, and spritesheets

## MCP tools

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

`analyze_reference`, `draw_with_dither`, `downsample_image`, `get_palette`, `set_palette`, `set_palette_color`, `add_palette_color`, `sort_palette`, `apply_shading`, `analyze_palette_harmonies`, `suggest_antialiasing`

Analyze references, apply dithering, downsample images, edit palettes, shade, and antialias artwork.

### Transform

`flip_sprite`, `rotate_sprite`, `scale_sprite`, `crop_sprite`, `resize_canvas`, `apply_outline`

Transform, resize, crop, and outline sprites.

### Animation

`add_frame`, `delete_frame`, `set_frame_duration`, `create_tag`, `delete_tag`, `duplicate_frame`, `link_cel`

Manage frames, timing, tags, and linked cels.

### Inspection and file operations

`get_pixels`, `export_sprite`, `export_spritesheet`, `import_image`, `save_as`

Verify pixels and work with PNG, GIF, JPG, BMP, spritesheets, and Aseprite files.

## Configuration

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

## Connect an MCP client

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

## License

[MIT](LICENSE)

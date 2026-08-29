# pixel-mcp

[English](README.en.md) · [기존 README 백업](README.original.md)

pixel-mcp는 AI 클라이언트가 MCP(Model Context Protocol)를 통해 Aseprite를 제어할 수 있게 해주는 로컬 MCP 서버입니다.

> 이 공개 포크는 OpenAI Codex를 활용해 개발하고 검증합니다.

## 주요 기능

- 캔버스, 레이어, 프레임 생성 및 관리
- 픽셀과 기본 도형 그리기
- 팔레트, 선택 영역, 애니메이션 작업
- 스프라이트 정보와 픽셀 색상 조회
- PNG, GIF, 스프라이트시트 내보내기

## 제공 MCP 도구

### 캔버스와 레이어

`create_canvas`, `add_layer`, `delete_layer`, `flatten_layers`, `get_sprite_info`

캔버스·레이어를 만들고 삭제하거나 병합하며 스프라이트 정보를 조회합니다.

### 드로잉

`draw_pixels`, `draw_line`, `draw_contour`, `draw_rectangle`, `draw_circle`, `fill_area`

개별 픽셀, 선, 윤곽선, 도형과 채우기를 지원하며 팔레트 색상 스냅을 선택할 수 있습니다.

### 선택 영역과 클립보드

`select_rectangle`, `select_ellipse`, `select_all`, `deselect`, `move_selection`, `cut_selection`, `copy_selection`, `paste_clipboard`

선택 영역과 클립보드 상태는 연속된 MCP 호출 사이에서도 유지됩니다.

### 픽셀 아트와 팔레트

`analyze_reference`, `draw_with_dither`, `downsample_image`, `quantize_palette`, `get_palette`, `set_palette`, `set_palette_color`, `add_palette_color`, `sort_palette`, `apply_shading`, `apply_auto_shading`, `analyze_palette_harmonies`, `suggest_antialiasing`

레퍼런스 분석, 디더링, 다운샘플링, 색상 양자화, 팔레트 편집, 자동 셰이딩과 안티앨리어싱을 제공합니다.

### 변형

`flip_sprite`, `rotate_sprite`, `scale_sprite`, `crop_sprite`, `resize_canvas`, `apply_outline`

스프라이트 변형, 크기 조절, 자르기와 외곽선 효과를 지원합니다.

### 애니메이션

`add_frame`, `delete_frame`, `set_frame_duration`, `create_tag`, `delete_tag`, `duplicate_frame`, `link_cel`

프레임, 재생 시간, 태그와 연결된 cel을 관리합니다.

### 조회와 파일 입출력

`get_pixels`, `export_sprite`, `export_spritesheet`, `import_image`, `save_as`

픽셀을 검증하고 PNG·GIF·JPG·BMP·스프라이트시트 및 Aseprite 파일을 입출력합니다.

## 요구 사항

- Go 1.25+
- Aseprite 1.3.17.2+ (1.3.18.3 권장)

## 설정

```json
{
  "aseprite_path": "/Applications/Aseprite.app/Contents/MacOS/aseprite",
  "temp_dir": "/tmp/pixel-mcp",
  "timeout": 30,
  "log_level": "info"
}
```

설정 파일 선택 우선순위는 다음과 같습니다.

1. `--config <path>`
2. `PIXEL_MCP_CONFIG`
3. `~/.config/pixel-mcp/config.json`

## MCP 클라이언트 연결

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

## 라이선스

[MIT](LICENSE)

# pixel-mcp

[기능 지원 현황 · 로드맵](docs/CAPABILITIES.md) · [Roadmap](docs/ROADMAP.md) · [Changelog](CHANGELOG.md) · [실행 환경 검사](docs/CAPABILITIES.md#실행-환경-사전-검사-gap-10) · [위험 작업 경고 계약](docs/WARNINGS.md)

[English](README.en.md) · [기존 README 백업](README.original.md)

pixel-mcp는 AI 클라이언트가 MCP(Model Context Protocol)를 통해 Aseprite를 제어할 수 있게 해주는 로컬 MCP 서버입니다.

이 공개 포크는 OpenAI Codex를 중심으로 개발하며, 아래 시작 안내도 Codex 기준입니다. 서버는 표준 MCP stdio를 사용하므로 다른 호환 클라이언트에서도 연결할 수 있습니다.

이 README는 `develop` 기준입니다. 포크의 수정과 복구 기능은 **Unreleased**이며 기존 `v0.5.0` 배포에 포함됐다는 뜻은 아닙니다. 최신 개발 기능을 사용하려면 아래 소스 빌드 절차를 따르세요.

## 주요 기능

- 캔버스, 레이어, 프레임 생성 및 관리
- 픽셀과 기본 도형 그리기
- 팔레트, 선택 영역, 애니메이션 작업
- 스프라이트 정보와 픽셀 색상 조회
- PNG, GIF, 스프라이트시트 내보내기
- 위험 작업 경고, dry-run, snapshot/restore 및 선택적으로 활성화하는 작업 이력/undo

## 제공 MCP 도구

기준 `develop`에는 PR #31까지 58개 도구가 있고, 이번 레이어·그룹 후보는 신규 3개를 포함해 61개를 등록합니다 (신규 도구 미병합/Unreleased). 세부 지원 범위와 제한은 [기능 지원 현황](docs/CAPABILITIES.md)을 참고하세요.

### 캔버스와 레이어

`create_canvas`, `add_layer`, `delete_layer`, `flatten_layers`, `get_sprite_info`, `set_layer_properties`, `move_layer`, `create_layer_group`

캔버스·레이어를 만들고 삭제하거나 병합하며 스프라이트 정보를 조회합니다. [레이어·그룹 편집](docs/LAYER_PROPERTIES.md)은 최신 구조 revision으로 이름·가시성·잠금·raster opacity/blend·순서·그룹 생성/이동을 지원합니다. 변경 후 구조 ID를 다시 조회합니다. 그룹 opacity/blend·특수 레이어·recursive 삭제는 새 도구에서 제외합니다.

### 드로잉

`draw_pixels`, `draw_line`, `draw_contour`, `draw_rectangle`, `draw_circle`, `fill_area`

개별 픽셀, 선, 윤곽선, 도형과 채우기를 지원하며 팔레트 색상 스냅을 선택할 수 있습니다.

### 선택 영역과 클립보드

`select_rectangle`, `select_ellipse`, `select_all`, `deselect`, `move_selection`, `cut_selection`, `copy_selection`, `paste_clipboard`

선택 영역과 클립보드 상태는 연속된 MCP 호출 사이에서도 유지됩니다.

### 픽셀 아트와 팔레트

`analyze_reference`, `draw_with_dither`, `downsample_image`, `quantize_palette`, `get_palette`, `set_palette`, `set_palette_color`, `add_palette_color`, `sort_palette`, `apply_shading`, `apply_auto_shading`, `analyze_palette_harmonies`, `suggest_antialiasing`

레퍼런스 분석, 디더링, 다운샘플링, 색상 양자화, 팔레트 편집, 자동 셰이딩과 안티앨리어싱 제안을 제공합니다.

Indexed sprite의 `apply_auto_shading`은 기존 palette index와 transparent index를 보존합니다. 새 shade 색상은 palette에 여유가 있을 때만 추가하며, 더 추가할 수 없으면 원본 pixel index를 유지합니다.

### 변형

`flip_sprite`, `rotate_sprite`, `scale_sprite`, `crop_sprite`, `resize_canvas`, `apply_outline`

스프라이트 변형, 크기 조절, 자르기와 외곽선 효과를 지원합니다.

### 애니메이션

`add_frame`, `delete_frame`, `set_frame_duration`, `create_tag`, `delete_tag`, `duplicate_frame`, `link_cel`, `set_cel_properties`

프레임, 재생 시간, 태그와 Aseprite native linked cel을 관리합니다. `link_cel`은 저장 후에도 source/target이 같은 image를 공유하며, 데이터 보호를 위해 target cel이 이미 있으면 실패합니다.

### 조회와 파일 입출력

`get_sprite_structure`, `get_pixels`, `export_sprite`, `export_spritesheet`, `import_image`, `save_as`

[상세 구조 조회](docs/SPRITE_STRUCTURE.md)는 계층과 frame별 cel 존재·위치·크기·opacity·native linked 관계를 읽기 전용으로 반환합니다. 구조 ID는 영구 ID가 아닙니다. [cel 편집](docs/CEL_PROPERTIES.md)은 조회의 `revision`을 필수 사전조건으로 받아 위치·opacity를 수정하며, linked 집합 전체 변경은 명시적 `allow_linked=true`가 필요합니다. 다른 편집 도구의 입력은 그대로입니다. 레이어 페이지(최대 100)와 frame 범위(최대 100)를 지원합니다.

픽셀을 검증하고 PNG·GIF·JPG·BMP·스프라이트시트 및 Aseprite 파일을 입출력합니다.

### 복구와 작업 이력 (Unreleased)

`create_snapshot`, `list_snapshots`, `restore_snapshot`, `delete_snapshot`, `list_operation_history`, `undo_last_operation`

저장된 파일의 snapshot을 만들고 복원할 수 있습니다. 복원 전에는 백업을 생성하며 기본 보존 기간은 7일, 저장소 한도는 100개/512 MiB입니다. 선택 설정인 절대 경로 `snapshot_dir`로 저장 위치를 바꿀 수 있습니다. 기본 위치는 `os.UserConfigDir()/pixel-mcp/snapshots`이며 `temp_dir`와 별개입니다. [복구 계약과 제한](docs/SNAPSHOTS.md)을 참고하세요.

설정에 `"enable_history": true`를 추가하면 성공한 제자리 편집을 자동 기록합니다(기본값 false). `list_operation_history`에 `sprite_path`를 전달해 확인한 뒤, 같은 경로와 최신 적용 작업의 `expected_operation_id`로 `undo_last_operation`을 호출합니다. 외부 변경과 오래된 ID는 거부하며, 자동 snapshot도 같은 용량·만료 제한을 공유합니다. [작업 이력 계약](docs/HISTORY.md)을 참고하세요.

## Codex 빠른 시작

### 1. 요구 사항과 소스 빌드

- Go 1.25+ (소스 빌드 시)
- 로컬 Aseprite 실행 파일: 1.3.17.2 이상 및 API 39 이상. 검증 버전은 [지원 현황](docs/CAPABILITIES.md#실행-환경-사전-검사-gap-10)을 참고하세요.
- MCP 서버를 실행할 환경에 설치된 Codex CLI

```bash
git clone --branch develop https://github.com/smrgd88/pixel-mcp.git
cd pixel-mcp
go build -o bin/pixel-mcp ./cmd/pixel-mcp
```

이후 예시의 `/absolute/path/to/pixel-mcp`는 방금 만든 `bin/pixel-mcp`의 실제 절대 경로로 바꾸세요. 실행 파일을 유지할 경로를 등록하세요.

### 2. 서버 설정 파일 작성

다음 JSON을 `/absolute/path/to/config.json` 같은 원하는 위치에 저장하세요. 아래 Aseprite 경로와 임시 디렉터리는 macOS 예시이며 실제 환경에 맞게 바꿔야 합니다. Aseprite 실행 파일 경로는 자동 탐색하지 않습니다.

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

### 3. Aseprite 실행 환경 확인

```bash
/absolute/path/to/pixel-mcp --config /absolute/path/to/config.json --health
```

정상일 때 종료 코드 0과 `success: true`인 JSON을 반환합니다. 이 검사는 Aseprite 버전/API 조건을 확인하며 Codex 연결 자체를 검사하지는 않습니다.

### 4. Codex에 MCP 서버 등록

다음 CLI 명령 또는 TOML 설정 중 하나를 사용하세요.

```bash
codex mcp add pixel-mcp -- /absolute/path/to/pixel-mcp --config /absolute/path/to/config.json
codex mcp list
codex mcp get pixel-mcp
```

직접 설정하려면 `~/.codex/config.toml`에 아래 항목을 추가하세요. 기존 설정은 유지합니다.

```toml
[mcp_servers.pixel-mcp]
command = "/absolute/path/to/pixel-mcp"
args = ["--config", "/absolute/path/to/config.json"]
```

JSON은 pixel-mcp 서버 설정이고 TOML은 Codex의 서버 실행 설정입니다. 설정 후 Codex CLI의 새 세션에서 `/mcp`로 활성 서버를 확인하세요. 등록 목록에 있다는 것만으로 Aseprite 도구 실행이 검증된 것은 아닙니다. [공식 Codex MCP 안내](https://developers.openai.com/codex/mcp)를 참고하세요.

서버는 Codex가 stdio로 실행합니다. 등록 인자에 `--health`를 넣으면 검사 후 종료되므로 위 등록 예시처럼 `--config`만 전달하세요. Codex 실행 환경에서 서버·설정·Aseprite 및 작업 파일 경로에 접근할 수 있어야 합니다.

### 5. 첫 작업 요청

Codex에 실제 저장 위치를 포함해 요청하세요.

> pixel-mcp로 32×32 RGB 캔버스를 만들고 빨간 원을 그려줘. 새 파일 `/absolute/path/to/art/demo.aseprite`에 저장하고 `/absolute/path/to/art/demo.png`로 내보낸 뒤 결과를 확인해줘.

감색 등 파괴적 작업에는 [warnings](docs/WARNINGS.md)와 지원 도구의 [dry-run](docs/DRY_RUN.md)을 확인하고, 필요한 경우 snapshot을 먼저 만드세요. GUI의 undo 기록 대신 파일 기반 복구를 사용합니다.

## 다른 MCP 클라이언트

`mcpServers` JSON을 사용하는 클라이언트에서는 아래 예시를 해당 클라이언트의 설정 위치에 적용할 수 있습니다. Codex의 `config.toml`에는 이 JSON을 넣지 않습니다.

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

## 개발 참여

개발에 참여할 때는 [Codex 개발 지침](AGENTS.md)과 [테스트 안내](docs/TESTING.md)를 확인하세요.

## 라이선스

[MIT](LICENSE)

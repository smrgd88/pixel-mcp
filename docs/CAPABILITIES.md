# 기능 지원 현황

기준일: 2026-09-11 · 구현 기준: `develop`의 `1076166edf99f60e92bfd7fc4def261264930dcb`

[로드맵](ROADMAP.md) · [실행 체크리스트](NEXT_STEPS.md) · [변경 이력](../CHANGELOG.md)

## 기준과 상태

- 등록된 MCP 도구는 **50개**다. 서버의 `registerTools()`와 각 `Register*Tools()`의 실제 등록 이름을 대조했다.
- `지원`은 MCP 입력으로 제공됨을 뜻한다. 공식 API 전체 옵션 지원이나 모든 조합의 실행 검증을 뜻하지 않는다.
- 기능군은 `지원 / 부분 / 미지원 / 현재 구조 밖`, 개발 진행은 `예정 / 진행 / develop 반영 / 태그 포함`으로 구분한다.
- 최초 태그는 로컬 `v0.1.0`부터 `v0.5.0`까지 코드에 도구가 존재하는지 확인한 결과다. GitHub Release 게시나 배포 artifact 검증을 의미하지 않는다.
- `v0.5.0`에도 50개 도구가 있다. 이후 포크 수정은 `Unreleased`이며, 도구 존재와 수정 배포 여부를 혼동하지 않는다.
- 지원 정책: Aseprite 1.3.17.2 이상/API 39 이상. 기존 전체 검증 기록은 1.3.17.2 및 1.3.18.3이다. 이번 문서 작업에서 테스트를 재실행하지 않았다.
- API별 최소 도입 버전은 전수 검증하지 않았다. 프로젝트 지원 하한을 개별 API 도입 버전으로 해석하지 않는다.

## 공식 연동 방식

Aseprite 공식 자동화 인터페이스는 [Lua Scripting API](https://www.aseprite.org/api/)와 [CLI](https://www.aseprite.org/docs/cli/)다. [app.command](https://www.aseprite.org/api/app_command/)는 Lua에서 편집 명령을 호출한다. 별도 Go SDK를 연결하는 구조가 아니라, Go MCP 서버가 호출마다 `aseprite --batch --script` 프로세스를 실행한다. 프로젝트의 Go MCP SDK는 Aseprite SDK가 아니다.

공식 API 존재만으로 batch 지원을 단정하지 않는다. UI 전용 명령과 대화형 옵션은 구분해야 한다. 버전별 가용성은 [API changes](https://github.com/aseprite/api/blob/main/Changes.md)를 확인한다.

## MCP 도구 목록

각 행 ID는 도구 이름과 독립된 추적 키다. 이름을 변경해도 ID는 유지한다. 아래 전체 도구의 기본 상태는 `지원 / 태그 포함`이며, 수정 상태가 별도로 표시된 경우 현재 구현 기준은 develop이다. 변경 버전은 이번에 확인한 변경만 기재하며 전체 변경 이력은 아니다.

| ID | 기능군 | MCP 도구 | 최초 태그 | 확인된 후속 변경 | 코드 근거 |
| --- | --- | --- | --- | --- | --- |
| MCP-001 | 캔버스·레이어·프레임 | `create_canvas` | v0.1.0 | — | [canvas.go](../pkg/tools/canvas.go) |
| MCP-002 | 캔버스·레이어·프레임 | `add_layer` | v0.1.0 | — | [canvas.go](../pkg/tools/canvas.go) |
| MCP-003 | 캔버스·레이어·프레임 | `add_frame` | v0.1.0 | — | [canvas.go](../pkg/tools/canvas.go) |
| MCP-004 | 캔버스·레이어·프레임 | `get_sprite_info` | v0.1.0 | — | [canvas.go](../pkg/tools/canvas.go) |
| MCP-005 | 캔버스·레이어·프레임 | `delete_layer` | v0.1.0 | — | [canvas.go](../pkg/tools/canvas.go) |
| MCP-006 | 캔버스·레이어·프레임 | `delete_frame` | v0.1.0 | — | [canvas.go](../pkg/tools/canvas.go) |
| MCP-007 | 캔버스·레이어·프레임 | `flatten_layers` | v0.5.0 | Unreleased: optional warnings (작업 브랜치) | [canvas.go](../pkg/tools/canvas.go) |
| MCP-008 | 드로잉 | `draw_pixels` | v0.1.0 | Unreleased: cel 좌표 수정 (#1) | [drawing.go](../pkg/tools/drawing.go) |
| MCP-009 | 드로잉 | `draw_line` | v0.1.0 | — | [drawing.go](../pkg/tools/drawing.go) |
| MCP-010 | 드로잉 | `draw_contour` | v0.1.0 | — | [drawing.go](../pkg/tools/drawing.go) |
| MCP-011 | 드로잉 | `draw_rectangle` | v0.1.0 | — | [drawing.go](../pkg/tools/drawing.go) |
| MCP-012 | 드로잉 | `draw_circle` | v0.1.0 | — | [drawing.go](../pkg/tools/drawing.go) |
| MCP-013 | 드로잉 | `fill_area` | v0.1.0 | — | [drawing.go](../pkg/tools/drawing.go) |
| MCP-014 | 선택·클립보드 | `select_rectangle` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-015 | 선택·클립보드 | `select_ellipse` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-016 | 선택·클립보드 | `select_all` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-017 | 선택·클립보드 | `deselect` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-018 | 선택·클립보드 | `move_selection` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-019 | 선택·클립보드 | `cut_selection` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-020 | 선택·클립보드 | `copy_selection` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-021 | 선택·클립보드 | `paste_clipboard` | v0.1.0 | — | [selection.go](../pkg/tools/selection.go) |
| MCP-022 | 애니메이션 | `set_frame_duration` | v0.1.0 | — | [animation.go](../pkg/tools/animation.go) |
| MCP-023 | 애니메이션 | `create_tag` | v0.1.0 | — | [animation.go](../pkg/tools/animation.go) |
| MCP-024 | 애니메이션 | `duplicate_frame` | v0.1.0 | — | [animation.go](../pkg/tools/animation.go) |
| MCP-025 | 애니메이션 | `link_cel` | v0.1.0 | Unreleased: native link (#6) | [animation.go](../pkg/tools/animation.go) |
| MCP-026 | 애니메이션 | `delete_tag` | v0.1.0 | — | [animation.go](../pkg/tools/animation.go) |
| MCP-027 | 픽셀 조회 | `get_pixels` | v0.1.0 | — | [inspection.go](../pkg/tools/inspection.go) |
| MCP-028 | 변형 | `downsample_image` | v0.1.0 | — | [transform.go](../pkg/tools/transform.go) |
| MCP-029 | 변형 | `flip_sprite` | v0.1.0 | — | [transform.go](../pkg/tools/transform.go) |
| MCP-030 | 변형 | `rotate_sprite` | v0.1.0 | — | [transform.go](../pkg/tools/transform.go) |
| MCP-031 | 변형 | `scale_sprite` | v0.1.0 | Unreleased: optional warnings (작업 브랜치) | [transform.go](../pkg/tools/transform.go) |
| MCP-032 | 변형 | `crop_sprite` | v0.1.0 | — | [transform.go](../pkg/tools/transform.go) |
| MCP-033 | 변형 | `resize_canvas` | v0.1.0 | — | [transform.go](../pkg/tools/transform.go) |
| MCP-034 | 변형 | `apply_outline` | v0.1.0 | — | [transform.go](../pkg/tools/transform.go) |
| MCP-035 | 파일 입출력 | `export_sprite` | v0.1.0 | — | [export.go](../pkg/tools/export.go) |
| MCP-036 | 파일 입출력 | `export_spritesheet` | v0.1.0 | — | [export.go](../pkg/tools/export.go) |
| MCP-037 | 파일 입출력 | `import_image` | v0.1.0 | — | [export.go](../pkg/tools/export.go) |
| MCP-038 | 파일 입출력 | `save_as` | v0.1.0 | — | [export.go](../pkg/tools/export.go) |
| MCP-039 | 팔레트·shading | `get_palette` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-040 | 팔레트·shading | `set_palette_color` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-041 | 팔레트·shading | `add_palette_color` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-042 | 팔레트·shading | `sort_palette` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-043 | 팔레트·shading | `set_palette` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-044 | 팔레트·shading | `apply_shading` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-045 | 팔레트·shading | `analyze_palette_harmonies` | v0.1.0 | — | [palette_tools.go](../pkg/tools/palette_tools.go) |
| MCP-046 | 참조 분석 | `analyze_reference` | v0.1.0 | — | [analysis.go](../pkg/tools/analysis.go) |
| MCP-047 | 디더링 | `draw_with_dither` | v0.1.0 | v0.3.0: Floyd-Steinberg 추가 | [dithering.go](../pkg/tools/dithering.go) |
| MCP-048 | 감색 | `quantize_palette` | v0.4.0 | Unreleased: optional warnings (작업 브랜치) | [quantization.go](../pkg/tools/quantization.go) |
| MCP-049 | 자동 shading | `apply_auto_shading` | v0.4.0 | Unreleased: indexed index 보존 (#8) | [auto_shading.go](../pkg/tools/auto_shading.go) |
| MCP-050 | 안티앨리어싱 | `suggest_antialiasing` | v0.1.0 | — | [antialiasing.go](../pkg/tools/antialiasing.go) |

## 공식 기능군 대비 차이

GAP ID는 로드맵과 체크리스트에서 공통으로 사용한다. 미지원은 전용 MCP 도구 또는 입력이 없다는 뜻이며, 기존 파일을 열었을 때 그 데이터가 반드시 손실된다는 뜻은 아니다.

| ID | 공식 기능·근거 | 현재 상태와 제한 | 목표 |
| --- | --- | --- | --- |
| GAP-01 | [Layer](https://www.aseprite.org/api/layer/): 이름·가시성·잠금·opacity·blend mode·순서·그룹 | 부분: 추가·삭제·flatten만 제공, 속성과 그룹 편집 입력 없음 | R3 |
| GAP-02 | [Sprite](https://www.aseprite.org/api/sprite/), [Cel](https://www.aseprite.org/api/cel/): 문서 구조·cel 관리 | 부분: 기본 정보·픽셀 조회와 link 제공; 상세 계층·cel 목록 및 위치/opacity 편집·unlink 도구 없음 | R3 |
| GAP-03 | [Tag](https://www.aseprite.org/api/tag/): 애니메이션 태그 | 부분: 생성·삭제, forward/reverse/pingpong; 기존 태그 수정·상세 조회 도구 없음 | R3 |
| GAP-04 | [CLI](https://www.aseprite.org/docs/cli/): 색상 모드 변환 | 부분: 생성 시 모드 선택·감색 시 indexed 변환; 범용 RGB/grayscale/indexed 전환 없음 | R3 |
| GAP-05 | [CLI](https://www.aseprite.org/docs/cli/): export·sprite sheet | 부분: 4종 형식, 단일/전체 frame, 5종 sheet 배치·JSON; 태그/레이어/범위 선택 없음. trim/extrude는 false, 3종 padding은 동일 값 사용 | R4 |
| GAP-06 | [Slice](https://www.aseprite.org/api/slice/): bounds·pivot·nine-slice | 미지원 | R4 |
| GAP-07 | [Tileset](https://www.aseprite.org/api/tileset/), [Sprite](https://www.aseprite.org/api/sprite/): tilemap·tileset·tile | 미지원 | R5 |
| GAP-08 | [app.command](https://www.aseprite.org/api/app_command/): 밝기·대비·색상 곡선·convolution·despeckle | 부분: outline은 제공, 나열한 보정 명령은 미지원; batch 옵션 검증 필요 | R5 |
| GAP-09 | [Layer](https://www.aseprite.org/api/layer/), [Slice](https://www.aseprite.org/api/slice/): data·properties | 미지원: 내부 상태 저장과 별개로 범용 사용자 metadata 편집 도구 없음 | R5 |
| GAP-10 | [app](https://www.aseprite.org/api/app/): version·apiVersion | 부분: CLI health에서 버전 접근 확인, 구조화된 capability 응답·최소 API 사전 거부 없음 | R1 |
| GAP-11 | [Selection](https://www.aseprite.org/api/selection/): 선택 영역 연산 | 부분: 사각형·타원·전체·이동·4종 결합 모드 및 복사/붙여넣기; 선택 반전 등 확장 없음. 선택 저장 코드가 bounds 중심이므로 비사각형 mask의 호출 간 보존은 별도 검증 필요 | R5 |
| GAP-12 | [API](https://www.aseprite.org/api/): Brush·Tool·ColorSpace·Grid 등 | 부분/미지원: 기본 도형·색상 API는 내부 사용, 범용 brush/ink·색공간·grid 설정은 미노출 | R5 |
| GAP-13 | [API](https://www.aseprite.org/api/): Plugin·Dialog·Editor·Events·Timer·WebSocket 등 | 현재 구조 밖: GUI 확장·지속 세션 연동을 MCP로 제공하지 않음 | 별도 SPIKE 필요 |

이 표는 제품 기능군 비교다. 공식 상수·속성·메서드 전체를 분모로 한 API 커버리지 비율을 주장하지 않는다.

## 자체 기능과 안전성

`analyze_reference`, `analyze_palette_harmonies`, `apply_shading`, `apply_auto_shading`, `suggest_antialiasing`은 자체 분석·알고리즘과 공식 primitive를 조합한 기능이다. `quantize_palette`의 3종 알고리즘과 `draw_with_dither` 패턴 역시 공식 명령과 일대일로 동일하다고 간주하지 않는다.

| ID | 항목 | 현재 상태 | 책임·의존성 |
| --- | --- | --- | --- |
| SAFE-01 | 위험 작업 warnings | 구현 완료 / 작업 브랜치 (Unreleased) | 3개 도구·4개 코드, [계약과 검증 범위](WARNINGS.md); develop 병합 전, 실제 클라이언트 UI 미검증 |
| SAFE-02 | dry-run | 예정 | 임시 복사본·동일 실행 경로·정리 정책, R2. CLI preview만으로 Lua 결과 검증을 대체하지 않음 |
| SAFE-03 | snapshot/restore | 예정 | 파일 저장 primitive + ID·보존·복원 정책, R2 |
| SAFE-04 | operation history·undo | 예정 | SAFE-03 의존, R2. 호출마다 프로세스가 달라 native undo를 호출 간 복구로 사용하지 않음 |
| SAFE-05 | 동일 파일 동시 수정·저장 실패 보호 | 예정 | canonical path lock·atomic 교체·fault test, R2 |
| OPS-01 | CI 실행 버전 artifact | 부분: 로그 출력 구현, artifact 예정 | [CI의 Report tool versions](../.github/workflows/ci.yml)에서 Go/Aseprite 버전 출력; 별도 artifact 보존은 R0 |
| OPS-02 | native launcher·OS matrix | 예정 | 기존 Go 서버와 cross-build 설정 존재가 launcher 구현/Windows 동작 검증을 뜻하지 않음, R5 |
| OPS-03 | 오류·request ID·로그 계약 통일 | 예정 | 기존 로깅과 timeout 처리는 존재, 전체 응답 표준화는 R1/R2 |

## 검증 근거와 갱신 규칙

- 등록: [server.go](../pkg/server/server.go), 도구 표의 입력 schema/handler.
- 구현: [Aseprite 계층](../pkg/aseprite), 실제 Lua generator와 Go 알고리즘.
- 회귀 및 전체 검증 기록: [NEXT_STEPS](NEXT_STEPS.md), [TESTING](TESTING.md).
- `draw_pixels` 수정: PR #1 (`b1204b8`), native link: PR #6 (`941fe34`), indexed shading: PR #8 (`b444ff0`). 이 수정들은 기준일 현재 로컬 태그 `v0.5.0`에 포함되지 않는다.
- 기능 변경 PR은 이 표의 상태·제한·버전과 `CHANGELOG.md`의 Unreleased를 함께 갱신한다. 테스트 통과 근거가 없으면 검증 완료로 표시하지 않는다.
- develop 병합 시 `develop 반영`, 릴리스 태그 포함 확인 시 버전 기록. 배포 게시 상태는 별도로 확인한다.
- 도구 추가/삭제 시 등록 집합과 이 표를 양방향 대조하고 총 개수를 갱신한다. GAP/SAFE/OPS ID는 완료 후에도 유지한다.

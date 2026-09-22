# pixel-mcp 다음 작업 체크리스트

작성일: 2026-08-31 · 현황 동기화: 2026-09-22

현재 기준: `develop`의 `c049ad42e3a48656d4f011c8705f413441440d2a` (PR #15까지 병합 반영). BUG-04/05는 `fix/be-quantization-contract`에서 수정, 병합 대기.

[기능 지원 현황](CAPABILITIES.md) · [로드맵과 실행 순서](ROADMAP.md) · [변경 이력](../CHANGELOG.md)

이 문서는 상세 실행 체크리스트와 검증 이력을 유지한다. 우선순위·의존성은 ROADMAP, 실제 도구 지원 상태는 CAPABILITIES를 기준으로 한다. 아래 P0/P1/P2 분류는 기존 항목의 분류이며 전체 착수 순서를 뜻하지 않는다.

일상 검증 환경: Go 1.25.14, Aseprite 1.3.18.3, Linux amd64 Docker 순차 실행

하한 검증 이력: Aseprite 1.3.17.2 전체 검증 완료. 릴리스 직전 또는 명시적 요청이 없으면 반복 실행하지 않는다.

지원 기준: Aseprite 1.3.17.2 이상 (`app.apiVersion >= 39`)

최신 검증 버전: Aseprite 1.3.18.3 (2026-08-25 공식 릴리스, 전체 Docker 검증 완료)

## 판단 기준

- **공식 직접 지원**: Aseprite Lua API 또는 CLI가 기능을 직접 제공한다.
- **조합 구현**: 공식 API를 조합하되 안전성·상태·복구 정책은 pixel-mcp가 구현한다.
- **pixel-mcp 책임**: MCP 계약, 파일 관리, 이력, dry-run처럼 Aseprite가 완성형 기능으로 제공하지 않는다.

공식 확인 자료:

- [Aseprite Lua API](https://www.aseprite.org/api/)
- [Aseprite API changes](https://github.com/aseprite/api/blob/main/Changes.md): `app.apiVersion`별 기능과 동작 변경을 제공한다.
- [Aseprite releases](https://github.com/aseprite/aseprite/releases): 지원 최소/최신 패치 버전을 결정하는 기준이다.
- [`app.transaction()`](https://www.aseprite.org/api/app/#apptransaction): 한 프로세스 안에서 변경을 하나의 undo/redo 단위로 묶는다.
- [Cel API](https://www.aseprite.org/api/cel/)와 [Sprite API](https://www.aseprite.org/api/sprite/): cel 위치·이미지·프레임·태그·저장을 제공한다.
- [`app.command.LinkCels`](https://www.aseprite.org/api/app_command)와 [`app.range`](https://www.aseprite.org/api/range/): timeline 선택 범위의 cel을 Aseprite native link로 연결한다.
- [Image API](https://www.aseprite.org/api/image/), [Palette API](https://www.aseprite.org/api/palette/), [`app.pixelColor`](https://www.aseprite.org/api/pixelcolor/): RGB/Grayscale/Indexed 픽셀과 palette index를 직접 다루는 기준이다.
- [`ChangePixelFormat`](https://www.aseprite.org/api/command/ChangePixelFormat/), [`ColorQuantization`](https://www.aseprite.org/api/command/ColorQuantization/), [`FlattenLayers`](https://www.aseprite.org/api/command/FlattenLayers/): 색상 모드 변환, palette 생성, layer 병합을 공식 명령으로 제공한다.
- [Aseprite CLI](https://www.aseprite.org/docs/cli/): batch 실행, Lua script, export와 metadata 출력을 제공한다.

> 중요: pixel-mcp는 현재 도구 호출마다 별도 `aseprite --batch` 프로세스를 실행하고 파일을 저장한다. 따라서 `app.transaction()`의 native undo stack은 MCP 호출 사이의 복구 수단이 아니다. 호출 간 undo는 snapshot/restore 같은 파일 기반 정책이 필요하다.

## 공식 지원 범위 검토 결과

문서 검토일: 2026-09-01

### 버전 정책

- **최소 지원 버전**: Aseprite 1.3.17.2, `app.apiVersion >= 39`
  - 현재 사용하는 핵심 pixel/cel/palette/save API는 더 오래된 버전에도 존재한다.
  - 그러나 1.3.17은 스크립트 권한 재사용 보안 문제를 수정했으므로 운영 지원 하한은 API 존재 여부가 아니라 보안 기준으로 정한다.
  - 태그 커밋 `793fb6526540b4c6f5cf8381ae189241cb003fdf`로 Docker build, `go vet`, unit/race/coverage와 전체 integration을 검증했다. `pkg/tools` integration은 58.912초였다.
- **최신 검증 버전**: Aseprite 1.3.18.3
  - 1.3.18.3은 Lua API 변경 없이 decoder와 플랫폼 버그를 수정한 최신 공식 패치다.
  - Docker build, `go vet`, unit/race/coverage와 전체 integration 검증을 완료했다.
- **재현 전용 버전**: Aseprite 1.3.15.5
  - upstream #16 보고 환경을 재현하는 데만 사용한다.
  - 보안 하한보다 낮으므로 정상 지원 또는 릴리스 승인 대상으로 취급하지 않는다.
- batch 실행에서는 `app.isUIAvailable == false`이므로 UI가 필요한 command/dialog 경로는 지원하지 않는다. 사용하는 command는 비대화형 옵션을 명시해야 한다.

### 기능별 판정과 구현 경계

| 기능 | 공식 지원 근거 | 판정 | pixel-mcp 구현 범위 |
| --- | --- | --- | --- |
| cel 좌표와 pixel 읽기/쓰기 | `Cel.position`, `Cel.bounds`, `Cel.image`, `Image:getPixel()`, `Image:drawPixel()` | 공식 직접 지원 | Image 좌표가 cel 기준 상대 좌표라는 규칙을 sprite 절대 좌표로 변환하고 범위 밖/no-cel 경로를 검증한다. |
| RGB/Grayscale/Indexed pixel 해석 | `Image.colorMode`, `ImageSpec.transparentColor`, `Sprite.transparentColor`, `app.pixelColor`, `Palette` | 조합 구현 | 공식 API는 index와 palette primitive를 제공한다. RGBA→palette index 선택, transparent index 보존, palette 확장/제한 정책은 pixel-mcp가 결정한다. |
| 색상 모드 변환과 quantization | `ChangePixelFormat`, `ColorQuantization`, `Palette:setColor()` | 조합 구현 | 변환 자체는 공식 명령을 사용한다. 알고리즘 선택, 색상 수, alpha, warning, 원본 보존과 결과 계약은 pixel-mcp 책임이다. |
| layer 병합 | `Sprite:flatten()`, `FlattenLayers{visibleOnly}` | 공식 직접 지원 | 대상/visible-only 정책, destructive warning, dry-run과 snapshot 연계를 추가한다. |
| 한 호출 안의 atomic 변경 | `app.transaction()` | 공식 직접 지원 | 한 Lua 실행 안의 변경을 transaction으로 묶고 오류 시 rollback한다. 저장 성공 여부와 프로세스 종료 실패는 별도로 처리한다. |
| MCP 호출 간 undo | `app.undo()`와 `Sprite.undoHistory`는 열린 sprite/process 안에서만 유효 | pixel-mcp 책임 | 호출별 새 batch 프로세스에서는 native undo를 사용하지 않는다. snapshot ID와 operation history로 복구한다. |
| snapshot/restore | `Sprite:saveCopyAs()`와 `Sprite:saveAs()` | 조합 구현 | 파일 복사는 공식 저장 API를 사용할 수 있다. UUID, atomic replace, 권한/symlink/path 검증, TTL/용량 제한은 Go 계층에서 구현한다. |
| export와 metadata | Sprite/Image save API와 Aseprite CLI export 옵션 | 조합 구현 | frame/layer 선택과 렌더링은 공식 기능을 사용한다. 출력 이름, overwrite, 임시 파일, 결과 JSON과 오류 계약은 pixel-mcp가 관리한다. |
| Aseprite capability 검사 | `app.version`, `app.apiVersion` | 공식 직접 지원 | health 응답에 두 값을 노출하고 최소 버전/API 미달을 Lua 실행 전 명시적 오류로 반환한다. |
| warning, dry-run, operation history | 완성형 공식 API 없음 | pixel-mcp 책임 | MCP output schema, 임시 복사본 실행, redaction, 보존/정리 정책을 구현한다. |
| 동일 파일 동시 수정과 cross-process 복구 | 완성형 공식 API 없음 | pixel-mcp 책임 | canonical path lock, timeout/cancel, 임시 파일과 atomic rename/fallback을 Go 계층에서 구현한다. |

### 구현 순서 기준

[ROADMAP의 다음 작업 순서](ROADMAP.md#다음-작업-순서)를 따른다. warnings(SAFE-01)은 PR #11로 병합했고 capability(GAP-10)는 PR #13으로 병합했고 파일 변경 보호(SAFE-05)는 PR #14로 병합했다. 현재 BUG-04/05 수정 브랜치를 검증하며, 다음 구현 순서는 BUG-01 → BUG-03 → BUG-02다. capability는 복구 기능 전에 완료한다. per-file lock/atomic save는 snapshot·undo 이후의 부가 작업이 아니라 복구 기능의 선행 기반으로 옮긴다. indexed helper 공통화는 색상 모드 전환 확장 전에 검증한다.

## 현재 우선 작업 — 알려진 동작 오류

2026-09-22, 사용자 보고 5건을 최신 develop `7e60ad9`에서 재확인했다. 아래 develop 결과는 수정 전 재현 이력이며, BUG-04/05 수정 브랜치의 상태는 완료 조건과 CAPABILITIES에 별도로 기록한다. 기존 dry-run보다 실제 결과/계약 오류를 우선한다. 전체 파일 접근 선언 리팩터링을 선행 조건으로 추가하지 않는다.

환경: Linux amd64 Docker, Go 1.25.14, Aseprite `1.3.18.3-dev` / API 41. 컨테이너 내부 `/tmp`의 격리 fixture, 실제 MCP 호출과 별도 Aseprite 렌더링 검사. 이번 재현은 이 환경의 아래 조건에 한정한다.

| ID / 보고 번호 | 우선도·실행 순서 | 최신 develop 결과 | 원인 확인 수준 |
| --- | --- | --- | --- |
| BUG-04 / 4 | P1, 1순위 (BUG-05와 묶음) | 불투명 빨강/파랑 각 32px인 RGB 8×8 이미지를 2색 indexed로 변환하면 빨강 64px 단색. 응답 palette/quantized_colors는 2 | 수정 브랜치: ChangePixelFormat의 mask 0 예약 + palette resize의 mask clamp를 피하고 명시적 index remap |
| BUG-05 / 5 | P1 계약 확인, 1순위 (BUG-04와 묶음) | RGB 4색 각 16px → dither=false/convert_to_indexed=false/target_colors=2 후 팔레트는 2개, 실제 픽셀은 원래 4색 유지 | 수정 브랜치: palette만 설정하던 Lua 경로에 cel remap 추가. false는 입력 모드 유지이며 감색은 수행 |
| BUG-01 / 1 | P1, 2순위 | bayer_2x2에서 density 생략과 명시적 0 모두 빨강 32/파랑 32. density=1은 파랑 64px | Go float64가 생략과 0을 구분하지 못하고 density==0을 0.5로 대체함 |
| BUG-03 / 3 | P1, 3순위 | 2-frame PNG export는 현재 MCP에서 오류, 최종 생성 파일 없음. 직접 Aseprite export는 raw1.png/raw2.png 생성 | 단일 requested path 응답과 이미지 시퀀스 저장 방식 불일치. SAFE-05가 base file 누락을 거부하므로 예전 성공 응답 형태와 다름 |
| BUG-02 / 2 | P2, 4순위 | PNG/JPG/GIF 분석 성공. 같은 fixture의 BMP와 .aseprite는 image: unknown format | 광고된 5개 형식과 Go image.Decode 등록 decoder(png/jpeg/gif) 불일치; Aseprite 변환 fallback 없음 |

P1은 무조건 운영 장애라는 뜻이 아니라, 이 작업 묶음에서 잘못된 수정 결과 또는 핵심 workflow 실패를 우선 처리한다는 분류다. BUG-05의 수정 계약은 [CAPABILITIES](CAPABILITIES.md#감색-계약-bug-0405-수정-브랜치)에 기록한다. 팔레트 항목 수와 실제 렌더링 색 수를 같다고 가정하지 않는다. BUG-02는 dry-run의 기술적 선행 조건은 아니므로 독립적으로 처리할 수 있다.

### 수정 작업과 완료 조건

1. `[BE][FIX] 감색 결과 및 옵션 계약 수정` — `fix/be-quantization-contract`, BUG-04/05.
   - [x] RGB 유지도 픽셀 remap을 수행하며, 투명 항목은 target_colors에 포함한다. indexed의 미사용 mask index와 최대 255개 불투명 항목 규칙을 문서화한다.
   - [x] 2색 indexed 단색화를 고치고, 응답의 색 수·팔레트와 실제 저장/렌더링 결과를 각각 검증한다.
   - [x] opaque/transparent, RGB/grayscale/indexed, dither/convert_to_indexed 조합을 실제 Aseprite로 회귀 검증한다.
   - [ ] PR을 develop에 병합한다 (이번 작업은 PR 생성까지만).
2. `[BE][FIX] 명시적 dithering density 0 보존` — `fix/be-dither-density-zero`, BUG-01.
   - [ ] 생략/0/0.5/1을 구분하고 허용 범위·endpoint 색상 계약을 고정한다.
   - [ ] 실제 MCP 입력으로 기본값과 endpoint의 저장 픽셀을 확인한다.
3. `[BE][FIX] 다중 프레임 PNG 출력 계약 수정` — `fix/be-multiframe-png-export`, BUG-03.
   - [ ] 실제 생성 파일 목록과 응답 경로를 일치시키고 단일 파일 응답의 호환성을 정의한다.
   - [ ] staging에서 생성된 모든 필요한 frame 파일의 publish·실패 정리·기존 출력 보존 정책을 정한다.
   - [ ] 1/여러 frame, 기존 target 유무, 경로/파일 수/이미지 내용과 source 보존을 검증한다.
4. `[BE][FIX] 참조 이미지 지원 형식 일치` — `fix/be-reference-format-support`, BUG-02.
   - [ ] 광고한 형식을 실제 지원하거나 지원 계약을 명시적으로 조정한다. native/animation 입력에서 분석할 frame 규칙도 기록한다.
   - [ ] PNG/JPG/GIF/BMP/.aseprite의 동일 fixture와 잘못된 파일을 검증한다.

BUG-04/05 브랜치 검증 (2026-09-22): Go 1.25.14 / Linux amd64 / Aseprite 1.3.18.3-dev에서 `go build ./...`, `go vet ./...`, `go test -race -cover ./...`, `go test -tags=integration ./...` 통과. pkg/tools integration 186.491초. darwin amd64/arm64, linux amd64/arm64, windows amd64 빌드 통과. 2색 저장·재열기 및 PNG export/reopen, RGB 픽셀 감색, 3종 알고리즘, grayscale/indexed 입력 모드 유지, 256색 경계, 투명 픽셀·반복 감색, cel 메타데이터, 오류 시 원본 바이트 보존과 기존 warnings를 확인했다. 새 변경의 최소 지원 Aseprite 1.3.17.2 실검증과 macOS/Windows native 실행은 수행하지 않았다. 하한 로컬 이미지의 버전 오보고 제약은 GAP-10 기록을 따른다.

공통: 최신 develop에서 원인별 수정·회귀 테스트, 기존 warnings 계약 유지, NEXT_STEPS/CAPABILITIES/CHANGELOG 갱신, PR 생성 후 병합 대기. 플러그인 저장소는 수정하지 않는다. 위 브랜치 분리는 권장 실행 단위이며 모든 수정이 완료됐다는 뜻이 아니다.

### 재현 입력 요약

- density: `draw_with_dither`, 8×8 region, Layer 1/frame 1, color1=#FF0000/color2=#0000FF, pattern=bayer_2x2. density 생략/0/1 대조.
- 감색: `quantize_palette`, target_colors=2, algorithm=median_cut, dither=false, preserve_transparency=true. 2색 opaque fixture에 convert_to_indexed=true; 4색 opaque fixture에 false. 저장 후 `Image(..., ColorMode.RGB):drawSprite(sprite, 1)`로 실제 RGBA 빈도 확인.
- 분석: 8색 8×8 동일 원본을 Aseprite로 각 형식에 저장한 뒤 `analyze_reference`, target_width/height=8, palette_size=5 호출.
- PNG: 2-frame native sprite에 `export_sprite`, format=png, frame_number=0, output_path=animation.png. 생성 파일 목록과 실제 응답 확인. raw `saveCopyAs`의 번호 붙은 PNG 생성도 대조.
- warnings: 감색 성공 응답의 기존 palette_quantization/color_mode_conversion 코드가 유지되는 것을 관찰했다. 수정 후에도 기존 경고 조건 회귀를 유지해야 한다.

추가 관찰: 최초 Docker 호스트 bind mount fixture에서는 일부 density control이 file_changed로 거부됐다. 동일 코드의 컨테이너 내부 filesystem 재현에서는 세 control 모두 성공했다. 원인은 미확정이며 이번 5건의 원인으로 단정하지 않는다. 파일시스템별 동작은 별도 확인 대상으로 기록한다.

## P0 — 다음 릴리스 전에 처리

### link_cel native linked cel 수정

완료된 작업: `[SHARED][FIX] link_cel native link 수정` (PR #6, develop 반영; 수정은 Unreleased)

- [x] `Sprite:newCel(..., srcCel.image, ...)`가 저장 후 독립 image 복사본을 만드는 문제를 재현한다.
- [x] 공식 `app.command.LinkCels`가 UI 전용 명령이 아니며 `app.range.layers/frames` 선택으로 batch mode에서 동작함을 확인한다.
- [x] save/reopen 뒤 source/target cel의 `image` identity와 cel 위치 보존을 검증한다.
- [x] source와 target 어느 쪽의 픽셀을 수정해도 다른 프레임에 반영되는 회귀 테스트를 추가한다.
- [x] 역방향 링크에서도 source의 기존 linked group, opacity와 user data를 보존한다.
- [x] invalid layer/source frame/target frame, 기존 target cel 거부와 실패 시 원본 보존을 검증한다.
- [x] Aseprite 1.3.17.2와 1.3.18.3에서 전체 검증을 완료한다 (`pkg/tools` integration 각각 60.172초, 56.541초).

완료 조건:

- [x] 두 지원 검증 버전에서 native link 회귀 테스트와 전체 integration이 통과한다.
- [x] build, `go vet`, unit/race/coverage가 통과하고 관련 변경만 커밋한다.

### CI를 실제 필수 검사로 만들기

`develop`과 `main`에 필수 `test` check가 적용됐고 PR #1, PR #6, PR #7, PR #8에서 통과했다. 남은 범위는 실행 버전 artifact 기록이다.

- [x] 포크 저장소에서 GitHub Actions 실행 여부와 권한을 확인한다.
- [x] PR마다 unit/race/coverage와 integration test가 `test` check로 표시되게 한다.
- [x] CI image는 포크 전용 `smrgd88/pixel-mcp-ci`를 우선 사용하고 공식 upstream image를 fallback으로 사용한다.
- [x] CI 설정을 `PIXEL_MCP_CONFIG` 기반 임시 파일로 전환해 `/root/.config` 의존성을 제거한다.
- [x] Aseprite와 Go 버전을 CI 로그에 남긴다 (`Report tool versions`).
- [ ] Aseprite와 Go 버전 기록을 CI artifact로 보존한다 (OPS-01).
- [x] unit/integration 실패 로그와 재현 명령을 GitHub Actions에서 확인할 수 있게 한다.

완료 조건:

- [x] PR 화면에 필수 `test` check가 나타나고 unit/race/coverage와 integration이 모두 통과한다.
- [x] 전역 사용자 설정 없이 CI가 재현된다.

### indexed auto shading 색상 반전 수정

대상: [upstream #11](https://github.com/willibrandon/pixel-mcp/issues/11)

Aseprite는 indexed image와 palette API를 공식 지원하지만 RGB 결과를 indexed image에 합성할 때의 palette index 보존 정책은 pixel-mcp가 명시해야 한다.

정책: 기존 palette index와 transparent index는 변경하지 않는다. 중복되지 않는 생성 색상은 palette에 여유가 있을 때만 뒤에 추가하고, 더 추가할 수 없을 때 non-exact shade는 원본 pixel index를 유지한다.

- [x] 기존 palette index와 픽셀 의미가 뒤바뀌는 재현 테스트를 먼저 추가한다.
- [x] 원본 영역별 색상 계열을 보존하는 index mapping 정책을 결정한다.
- [x] 새 shading 색상을 palette에 추가할지, 기존 색으로 제한할지 입력 계약에 명시한다.
- [x] indexed mode와 transparent index가 유지되는지 검증한다.
- [x] RGB 모드 결과와 indexed 모드 결과를 각각 실제 Aseprite로 검증한다.

완료 조건:

- [x] 나무의 줄기/잎처럼 서로 다른 색상 영역이 shading 뒤에도 뒤바뀌지 않는다.
- [x] `get_pixels`와 export PNG 양쪽에서 예상 RGBA가 일치한다.

### indexed 기본 작업의 검은 이미지 문제 재현

대상: [upstream #16](https://github.com/willibrandon/pixel-mcp/issues/16)

상태: **보류**. 보고 버전 1.3.15.5가 지원 하한보다 낮으므로 지원 버전에서 같은 문제가 확인되거나 릴리스 검증에 필요할 때 재개한다.

- [ ] 보고 환경 Aseprite 1.3.15.5에서 `create_canvas(indexed) → draw_circle → export`를 재현한다.
- [ ] 현재 권장 버전 Aseprite 1.3.18.3과 결과를 비교한다.
- [ ] palette index 0, `transparentColor`, shape tool의 색상 변환을 각각 기록한다.
- [ ] `get_pixels` 결과와 최종 PNG를 함께 검증한다.
- [ ] MCP 클라이언트 차이와 무관하게 동일한 raw tool call로 재현되는지 확인한다.

완료 조건:

- [ ] 지원 Aseprite 버전에서 indexed 빨간 원이 검은 사각형으로 export되지 않는다.
- [ ] 문제가 클라이언트 호출 순서인지 서버 렌더링인지 원인이 분리된다.

### draw_pixels 수정의 upstream 반영 준비

대상: [upstream #19](https://github.com/willibrandon/pixel-mcp/issues/19)

- [x] asymmetric non-zero cel 위치 `(10,6)` 회귀 테스트를 추가했다.
- [x] cel 바깥 좌표와 새 레이어/no-cel 경로를 검증했다.
- [x] Aseprite 1.3.18.2에서 전체 `pkg/tools` integration suite가 통과했다.
- [x] 이슈 보고 환경인 Aseprite 1.3.17.2에서도 회귀 테스트를 실행한다.
- [ ] upstream 제출 범위에서 설정 격리와 좌표 수정을 분리할지 결정한다.

완료 조건:

- [x] 지원 최소 버전과 최신 검증 버전에서 요청 좌표와 실제 RGBA가 일치한다.

## P1 — 안전한 MCP 작업 흐름

### 위험 작업 warning 반환

SAFE-01: PR #11로 develop `6c9ac5e`에 병합했으며 [응답 계약](WARNINGS.md)에 적용 범위와 검증 한계를 기록한다.

대상: [upstream #15](https://github.com/willibrandon/pixel-mcp/issues/15)

분류: **pixel-mcp 책임**

- [x] 공통 output contract에 `warnings`를 추가한다.
- [x] `quantize_palette`, `flatten_layers`, color mode 변경, 보간 scale에 warning을 적용한다.
- [x] stderr 로그와 MCP JSON warning의 역할을 구분한다.
- [x] warning 조건을 table-driven unit test로 고정한다.
- [x] MCP SDK contract test에서 optional schema, text/structured 응답, legacy JSON decode를 검증한다.
- [ ] 실제 Claude/Codex/Gemini 앱 UI의 optional field 수용·표시를 검증한다 (자동 protocol 검증과 구분).

### dry-run 설계

대상: [upstream #12](https://github.com/willibrandon/pixel-mcp/issues/12)

분류: **pixel-mcp 책임**

- [ ] dry-run이 원본 파일을 열거나 저장하지 않는 분석-only 경로인지 정의한다.
- [ ] Aseprite 실행이 필요한 preview는 원본이 아닌 임시 복사본에서 수행한다.
- [ ] `quantize_palette`, `apply_auto_shading`, `flatten_layers`부터 적용한다.
- [ ] dry-run 결과와 실제 적용 결과의 summary가 일치하는 회귀 테스트를 추가한다.
- [ ] 임시 파일 생성·실패·정리 정책을 문서화한다.

### snapshot/restore 도구

대상: [upstream #13](https://github.com/willibrandon/pixel-mcp/issues/13)

분류: **공식 저장 API를 이용한 조합 구현**

- [ ] `create_snapshot`, `restore_snapshot`, `delete_snapshot`, `list_snapshots` 계약을 먼저 정의한다.
- [ ] snapshot ID는 UUID로 만들고 sprite 경로와 metadata를 함께 저장한다.
- [ ] restore 전 현재 파일을 보호하는 2단계 교체 또는 추가 snapshot 정책을 정한다.
- [ ] atomic rename 가능 여부와 cross-volume fallback을 처리한다.
- [ ] 파일 권한, symlink, path traversal을 검증한다.
- [ ] 최대 개수·TTL·용량 제한과 cleanup을 구현한다.

### operation history와 undo

대상: [upstream #14](https://github.com/willibrandon/pixel-mcp/issues/14)

분류: **pixel-mcp 책임**, snapshot 기능에 의존

- [ ] native undo stack을 MCP 호출 간 undo로 사용하지 않는다.
- [ ] history entry schema와 민감한 경로/입력값 redaction 정책을 정한다.
- [ ] 성공한 변경만 history에 기록한다.
- [ ] snapshot ID와 operation ID를 연결한다.
- [ ] `undo_last_operation` 실패 시 원본을 보존한다.
- [ ] history/snapshot 불일치를 탐지하고 복구 가능한 오류로 반환한다.

### 동일 sprite 동시 수정 보호

SAFE-05: PR #14로 develop `7e60ad9`에 병합. [보호 범위·실패 경계](FILE_PROTECTION.md)를 기준으로 검증한다.

분류: **pixel-mcp 책임**

- [x] canonical sprite path 기준으로 per-file lock을 구현한다.
- [x] 같은 파일의 읽기/쓰기 및 쓰기/쓰기 동시 호출 정책을 정의한다.
- [x] timeout과 취소 시 lock이 누수되지 않는지 테스트한다.
- [x] 저장 중 프로세스 종료 시 원본 파일이 손상되지 않는지 fault test를 추가한다.

## P2 — 호환성과 운영성

### 공식 API capability와 버전 검사

GAP-10: PR #13으로 develop `897c2f1`에 병합했으며 [실행 환경 검사 계약](CAPABILITIES.md#실행-환경-사전-검사-gap-10)에 검증 범위를 기록한다.

- [x] health output에 `app.version`과 `app.apiVersion`을 포함한다.
- [x] 최소 지원 Aseprite 버전을 1.3.17.2/API 39로 문서에 명시한다.
- [x] 사용하는 기능별 공식 API와 pixel-mcp 구현 경계를 표로 관리한다.
- [x] 미지원 버전/API 하한은 도구 실행 전에 capability 오류로 반환한다.
- [x] 버전/API 경계·조회 실패·취소·원본 보존·health JSON을 검증한다.
- [ ] 정확한 버전을 보고하는 Aseprite 1.3.17.2 바이너리로 하한 실행 검증을 완료한다 (현재 로컬 이미지 보고값은 `1.x-dev`).
- [x] Aseprite API changes와 공식 release를 릴리스 체크리스트의 근거로 연결한다.

### cross-platform native launcher

대상: [upstream #18](https://github.com/willibrandon/pixel-mcp/issues/18)

분류: **Go 배포 계층**, Aseprite CLI와 별개

- [ ] launcher가 OS/arch별 서버 binary를 선택하고 모든 argv/stdin/stdout/stderr를 전달한다.
- [ ] Windows에서 WSL/Git Bash 의존성 없이 실행되는지 검증한다.
- [ ] darwin amd64/arm64, linux amd64/arm64, windows amd64를 cross-build한다.
- [ ] GoReleaser artifact에 server와 launcher를 함께 포함한다.
- [ ] 지원하지 않는 OS/arch 오류에 기대 binary 이름과 실제 플랫폼을 표시한다.

### 테스트 matrix 확대

- [ ] macOS arm64 외에 Linux와 Windows smoke test를 추가한다.
- [x] 최소 지원 버전과 최신 Aseprite 버전을 모두 검증한다.
- [ ] RGB, grayscale, indexed 각각에 공통 drawing/export contract test를 적용한다.
- [ ] palette index 0과 transparent index를 별도 축으로 테스트한다.
- [ ] non-zero cel, linked cel, group layer, hidden layer, multi-frame 조합을 추가한다.
- [ ] MCP client contract test에 Claude/Codex, Gemini 등 클라이언트별 JSON 호환성을 포함한다.

### 관측성과 오류 계약

- [ ] 모든 tool output에 일관된 `success`, `warnings`, `details` 규칙을 적용한다.
- [ ] Aseprite stderr, Lua 오류, timeout, validation 오류를 구분된 error code로 반환한다.
- [ ] request ID를 MCP 응답과 stderr 로그 양쪽에 연결한다.
- [ ] 경로·사용자 데이터가 기본 로그에 과도하게 노출되지 않게 한다.

## 권장 실행 순서

중복된 순서 목록 대신 [ROADMAP](ROADMAP.md#다음-작업-순서)을 기준으로 관리한다.

- 현재 작업: `[BE][FIX] 감색 결과 및 옵션 계약 수정` (BUG-04/05), 작업 브랜치 수정·검증 후 PR 병합 대기.
- 다음 구현 작업: `[BE][FIX] 명시적 dithering density 0 보존` (BUG-01).
- 이후 BUG-01 → BUG-03 → BUG-02를 처리하고, SAFE-02 dry-run → SAFE-03 snapshot/restore → SAFE-04 history/undo로 진행한다.
- 전체 파일 접근 선언 리팩터링은 확정된 선행 작업이 아니다. 버그 수정/dry-run에 필요한 범위만 정리한다.
- 다음 릴리스 전 운영 잔여: CI 버전 artifact(OPS-01), upstream #19 제출 범위 결정.
- 신규 편집·조회·export·slice·tilemap 후보는 [CAPABILITIES의 GAP 목록](CAPABILITIES.md#공식-기능군-대비-차이)과 ROADMAP R3–R5에서 추적한다.
- indexed #16은 기존 보류 조건을 유지한다.

## 병합 검증 이력

- [x] PR #1에 필수 `test` check가 나타나고 최신 `develop` 기준으로 통과했다.
- [x] 리뷰어가 config 우선순위와 기본 동작 호환성을 확인한다.
- [x] 리뷰어가 `draw_pixels`의 cel bounds 확장, local 좌표 변환과 indexed transparent index 처리를 확인한다.
- [x] Aseprite 1.3.17.2에서 Go 1.25.14, `go vet`, unit/race/coverage와 전체 integration을 검증한다.
- [x] PR #1을 `develop`의 `b1204b878ccc9e50e8ac1384266b626d8604bc8d`로 병합한다.
- [x] push/PR에는 Downloads의 생성 이미지나 로컬 임시 파일이 포함되지 않았는지 확인한다.
- [x] PR #6의 native `link_cel` 회귀 테스트와 필수 `test` check가 통과했다.
- [x] PR #7에서 CI 설정을 `PIXEL_MCP_CONFIG` 기반 임시 파일로 격리했다.
- [x] PR #8에서 indexed auto shading의 palette/transparent index, RGB 비회귀와 deterministic shade 순서를 검증했다.
- [x] PR #8을 `develop`의 `b444ff003e187aae8c18fbf04e51548ffbaa3370`으로 squash merge했다.

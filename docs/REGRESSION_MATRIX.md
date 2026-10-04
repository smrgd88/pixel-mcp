# 공통 회귀 조합 점검

작업: `[SHARED][TEST] 공통 회귀 조합 점검` · 2026-10-04

기준: `origin/develop` `65074051f5d3903124ead37367c7fc62d9e7e7f6`, PR #25 포함, Unreleased.
브랜치: `test/shared-regression-matrix`. 이 점검은 PR #27로 develop `ec99cc2`에 병합됐고 PR #28 문서도 `2ec6c79`에 병합됐다. 아래 기존 실행·리뷰 기록은 당시 이력이다.

RM-FIX-01 후속: `fix/be-nested-draw-pixels`에서 구현 및 성공 회귀로 전환했다. 루트 우회를 제거하고 nested 상태 그대로 검사한다. FIX는 PR #29로 develop `2ced64c`에 병합했으며 [계약·검증](DRAW_PIXELS_NESTED_FIX.md)을 따른다.

현재 통합 상태: PR #27은 develop `ec99cc2`에 병합 완료했고 PR #28도 `2ec6c79`에 병합 완료했다. 아래 기준·실행 기록은 회귀 작업 당시 이력이다. GAP-02 조회는 별도 feature 브랜치에서 구현하며 [계약·검증](SPRITE_STRUCTURE.md)을 따른다.

## 실제 backlog와 이번 범위

[NEXT_STEPS](NEXT_STEPS.md#테스트-matrix-확대)의 미완료 목록은 ① OS별 CI matrix ② RGB/grayscale/indexed 공통 drawing/export ③ palette index 0/transparent index ④ non-zero/linked cel, group/hidden layer, multi-frame ⑤ 클라이언트별 JSON 호환성이다. 최소 지원 버전 검증은 이미 완료 표시다. 이번 작업은 ②–④의 기존 증거 대조와 위험 조합 보강이며 ①·⑤ 전체 완료를 주장하지 않는다.

[ROADMAP](ROADMAP.md#다음-작업-순서)의 기존 순서는 OPS-03 → 이 점검 → R3 범위 선정이다. 이번 문서 동기화에서 OPS-03 PR #25 병합 완료와 본 작업의 PR #27 develop 병합 완료 상태를 반영했다. BUG-01–05, DITHER-01/02, SAFE-01–05, GAP-10 구현도 기준에 포함된다. 릴리스 완료를 뜻하지 않는다.

별도 문서 작업은 `shared-docs-post-ops-next-steps` 워크트리의 `docs/shared-post-ops-next-steps` 브랜치, 커밋 `0460cede14578840d8f4a7744370463d4e7a4afc`로 확인했다. 해당 커밋의 세 문서 동기화를 이번 브랜치에 반영하고 회귀 점검 결과까지 갱신했다. 별도 워크트리는 수정하지 않았다. 기존 광범위 matrix 체크박스는 일괄 완료 처리하지 않는다.

## 판정과 실행 근거 읽는 법

- **검증됨**은 아래 이름의 테스트가 명시적으로 assert하는 범위에 한정한다. 하나의 테스트가 여러 축을 가진다고 모든 도구의 전체 곱을 검증한 것은 아니다.
- **미검증**은 구현이 있어도 해당 교차 조합의 저장/재열기/렌더링 증거가 부족한 경우다.
- **지원 범위 밖**은 현재 계약이 명시적으로 거부하거나 MCP 입력이 없는 기능이다. Lua로 fixture를 만드는 것은 그 기능의 MCP 지원 증거가 아니다.
- `L`은 이번 Linux 전체 integration(기본 테스트 포함), `M`은 이번 macOS 새 matrix 테스트만을 뜻한다. 실제 실행 결과는 아래 실행 기록에 둔다. 기존 macOS/하한 검증 이력은 NEXT_STEPS의 해당 PR 기록이며 이번 전체 실행으로 계산하지 않는다.

## 축별 증거 지도

테스트 경로는 `pkg/tools/` 기준이다. 테스트명 뒤 괄호는 파일이다.

| 조합/계약 | 실제 테스트와 검증 내용 | 판정·환경 |
| --- | --- | --- |
| RGB/indexed, non-zero cel, drawing | `TestIntegration_DrawPixels_UsesSpriteCoordinatesForPositionedCel`, `...PositionedIndexedCel` (`drawing_pixels_coordinates_integration_test.go`): (10,6) 전제 확인, 밖으로 확장한 좌표와 기존 픽셀·indexed 투명 배경 조회 | 검증됨 L; grayscale 단독 기존 공백은 신규 행 참조 |
| RGB, off-canvas cel | `TestIntegration_DrawPixels_PreservesOffCanvasCelContent`: (-2,-2) 원래 픽셀과 새 (10,10) 보존 | 검증됨 L; 다른 모드의 음수 위치는 미검증 |
| RGB native link, non-zero, multi-frame | `TestIntegration_LinkCel_NativeLinkPersistsAndPropagates`, `...BackwardsFrameOrder` (`link_cel_integration_test.go`): 재열기 image identity, 양방향 변경, 위치·opacity·zIndex·data·tag 보존 | 검증됨 L |
| RGB linked drawing 확장 | `TestIntegration_DrawPixels_PreservesNativeLinkedCels`: frame 1 drawing 후 두 frame의 새/기존 픽셀 조회 | 검증됨 L; 신규 테스트는 frame 2에서 편집하고 재열기 identity도 확인 |
| indexed palette index 0 | `TestIntegration_DrawRectangle_GetPixels_PaletteIndex0Bug` (`drawing_rectangle_palette_index0_bug_test.go`): 빨강/초록 각 16px 조회 | 검증됨 L; 이 테스트만으로 native index identity/export 전체를 보장하지 않음 |
| indexed mask 0/3/255, palette resize | `TestAuditPaletteMaskResize` (`audit_fixes_integration_test.go`): set_palette 후 mask와 렌더링 배열 불변, add_palette_color 후 mask | 검증됨 L; linked/multi-frame resize 조합 미검증 |
| RGB/indexed 기본 drawing·dither | `TestIntegration_IndexedColorMode_DrawingVerification`, `TestIntegration_DrawWithDither_RGBMode`, `...IndexedMode` (`drawing_palette_integration_test.go`); `TestDitherDensityEndpointsAllPatterns`, `...IndexedEndpoints` (`dithering_density_integration_test.go`): 색/endpoint/영역 밖 보존 | 검증됨 L; grayscale 전 도형/dither 및 linked 도형 전체는 미검증 |
| RGB/indexed, group/hidden, non-zero, multi-frame PNG | `TestExportSequencePNGContract`, `TestExportSequenceIndexedPixels` (`export_sequence_integration_test.go`): 두 frame 색/alpha/크기, RGB 원본 바이트와 기존 출력·base·orphan 보존, files 경로/크기/번호 | 검증됨 L; indexed 행 자체에는 source bytes assert 없음(신규 보강) |
| JPG/BMP/GIF, multi-frame | `TestExportSequenceOtherFormatsAndSingleCanvas`: JPG/BMP signature·파일 수, GIF 재열기 frame 수, 선택 frame 단일 경로 | 검증됨 L(구조); grayscale/indexed 픽셀·GIF timing 전체 교차는 미검증 |
| RGB/grayscale/indexed, group/hidden, 위치·opacity, 분석 | `TestReferenceNativeCompositeModes` (`analysis_formats_integration_test.go`): native와 PNG brightness/edge 비교 및 원본 bytes 보존 | 검증됨 L; 기대 PNG가 같은 export generator를 사용하므로 독립 렌더링 oracle은 아님 |
| RGB/grayscale/indexed, 감색·투명 | `TestQuantizationContractPreservesInputModeAndTransparency`, `...GrayscaleReduction`, `...OpaquePixels` (`quantization_contract_integration_test.go`): 모드·투명/불투명·두 색 렌더링, grayscale 실제 remap, 알고리즘/dither 조합 | 검증됨 L; multi-frame 감색은 지원 범위 밖 |
| 감색 group/hidden/non-zero/metadata | `TestQuantizationContractRGBRemapsWithoutFlattening`, `...IndexedKeepsCelMetadata`: 그룹·위치·opacity/data와 indexed 변환 cel 속성 보존 | 검증됨 L; 모든 모드×계층 속성 전체 곱은 미검증 |
| dry-run, RGB 두 레이어 | `TestDryRunPreservesSourceAndMatchesApply` (`dry_run_integration_test.go`): bytes/inode/권한/mtime 보존, 반복 preview, 실제 apply summary 일치 | 검증됨 L; grayscale/indexed linked/group 전체 조합은 미검증 |
| snapshot/undo | `TestSnapshotMCPPreservesNativeSpriteAndRecoversEdit`, `TestHistoryMCPRealEditUndoAndExclusions`: 재시작 후 bytes 복원·재열기, history 제외 조건 | 검증됨 L; 세 모드 linked cel 렌더링 교차 복원은 미검증 |
| 실패·취소·출력 rollback | `TestExportSequenceInvalidDestinationPreservesFiles`, `...ValidationAndSchema`, `...StalePlanWritesNothing`; `pkg/aseprite/output_files_test.go`의 `TestOutputFilesRollbackOnCommitFailure`, `...CancelDuringPublication`, `...RollbackFailureRetainsBackup` | 검증됨 L; crash 원자성/자동 복구는 지원 범위 밖 |
| 신규: RGB/grayscale/indexed mask 0/3 × positioned native link × drawing→PNG | `TestRegressionMatrixLinkedDrawingExport/{rgb,grayscale,indexed-mask-zero,indexed-opaque-zero}` (`regression_matrix_integration_test.go`): 아래 상세 | 검증됨 L/M(명시된 경로); 그룹 내부 drawing도 FIX 브랜치에서 성공 검사; [후속 검증](DRAW_PIXELS_NESTED_FIX.md) |

## 위험 공백 선정과 신규 검증

같은 잘못된 palette index 또는 cel 좌표가 저장된 원본과 모든 linked frame으로 전파될 수 있어 이 교차 조합을 우선했다. 전체 곱 대신 4개 fixture를 사용한다.

1. 16×16 문서, (5,3)의 2×2 회색 cel, 두 frame의 native link를 만든다. 숨김 레이어와 숨김 그룹에는 전체 화면 흰색 cel을 넣어 visibility 오류가 픽셀 비교에서 드러나게 한다.
2. indexed는 mask=0/3을 나눠 설정하고 mask palette entry 자체는 불투명 magenta로 만든다. 따라서 palette alpha가 우연히 투명도를 대신하는 것을 막는다. mask=3에서는 흰색의 실제 index가 0인지도 직접 검사한다.
3. 재열기 후 link identity·위치·계층 전제를 확인한다. 현재 FIX에서는 그룹 내부 대상에 좌표 오류를 요청해 원본 bytes 보존을 확인한다.
4. paint를 그룹 안에 유지한 채 MCP draw_pixels로 frame 2의 cel 밖 (1,2)를 단일 요청으로 편집하고 재열기 identity·위치를 확인한다. 내부 (5,3)도 편집한 뒤 두 frame을 렌더링한다. 기존 PR #27에서는 조회 실패를 기록하고 루트로 이동하는 우회를 사용했으나 RM-FIX-01에서 제거했다.
5. 별도 프로세스로 재열어 모드·native link·두 cel 위치 일치·frame duration·sprite data·숨김 계층 존재 및 mask/빈 픽셀 index를 assert한다.
6. MCP export_sprite로 두 PNG를 생성하고 Go PNG decoder로 **각 256픽셀 전체**를 독립 상수 기대값과 비교한다. 비대상 3개의 회색 픽셀과 투명 배경, 두 흰 픽셀이 두 frame 모두 일치해야 한다. source bytes는 export 전후 동일해야 한다.

## 별도 FIX 인계: RM-FIX-01 (발견 당시 기록)

작업 후보: `[BE][FIX] 그룹 내부 draw_pixels 대상 조회 수정`, 신규 작업 시 `fix/be-nested-draw-pixels`.

- 기준 커밋 `6507405`, Linux 1.3.18.3-dev에서 네 fixture 모두 최초 성공 기대 테스트가 실패했다: `Layer not found: paint`.
- 재현: native 문서의 `visible-group/paint`에 positioned linked cel을 저장 → `draw_pixels(layer_name="paint", frame_number=2, pixels=[{x:1,y:2,color:"#FFFFFFFF"}])`.
- 원인 근거: `pkg/aseprite/lua_drawing.go`의 `DrawPixels`가 `ipairs(spr.layers)`로 최상위만 검색한다. 자식 이름은 존재하나 탐색되지 않는다. 다른 drawing 도구에도 유사 루프가 있지만 이번 재현을 그 도구들의 확정 결함으로 확대하지 않는다.
- 현재 영향: 그룹 안의 raster layer 편집이 거부됨. 이 재현에서는 원본 bytes 손상 없음. 그룹 편집 API 미지원(GAP-01)과 기존 그룹 내부 raster 편집 실패는 구분한다.
- 당시 테스트의 오류 기대는 **현상 기록**이었다. 현재 FIX는 성공 기대와 저장/재열기·render 검사로 전환했다. 중복/그룹/missing/hidden/locked 정책과 후속 검증은 [RM-FIX-01 보고서](DRAW_PIXELS_NESTED_FIX.md)에 기록한다.
- production 코드 변경은 하지 않았다. 알려진 실패를 skip하거나 정상 지원으로 보고하지 않는다.

## 잔여와 R3 첫 구현 후보

첫 후보는 **GAP-02 중 읽기 전용 상세 구조/cel 조회**다. 이번 결함에서 이름만으로 그룹 내부 대상을 고르는 한계가 드러났다. 계층 경로·layer 식별자, frame별 cel 존재/위치/opacity, linked 관계를 먼저 조회할 수 있어야 GAP-01 편집과 GAP-02 위치/opacity/unlink, R4 export 선택의 대상을 검증할 수 있다. 기존 get_sprite_info와 호환되는 별도/optional 계약, 재열기 기반 결과와 중복 이름 정책을 먼저 범위로 정한다. 회귀 작업 당시에는 후보였으나 사용자의 후속 지시로 별도 `feature/be-sprite-structure`에서 읽기 전용 조회를 구현하고 PR #30으로 develop `eca9e83`에 병합했다. [확정 계약](SPRITE_STRUCTURE.md); 위치·opacity는 [후속 구현](CEL_PROPERTIES.md), unlink 및 GAP-02 전체 완료는 후속이다.

| 실제 R3 목록 | 다음 판단 |
| --- | --- |
| GAP-01 레이어 속성·그룹 | RM-FIX-01 이름 지정 정책은 PR #29로 반영. 구조 ID 기반 편집은 별도 범위 선정 |
| GAP-02 상세 구조·cel 위치/opacity/unlink | 읽기 전용 조회 PR #30 develop 반영; 위치·opacity PR #31 develop 반영 ([CEL_PROPERTIES](CEL_PROPERTIES.md)), unlink 별도 진행 |
| GAP-03 태그 상세 조회·기존 태그 수정 | [별도 태그 작업](TAG_EDITING.md)에서 구현·후보 검증; 미병합, 본 공통 회귀 당시 검증과 별개 |
| GAP-04 범용 색상 모드 전환 | indexed helper 공통화와 transparent index 불변식 확대가 선행; 현재 4-case 통과만으로 충족하지 않음 |

남은 높은 위험 조합은 grayscale의 모든 도형·dither, linked cel의 팔레트 resize 및 snapshot/undo 복원 렌더링, 음수 위치 grayscale/indexed, blend/부분 alpha·layer/cel opacity·서로 다른 linked 위치, 프레임별 palette, 모든 export 형식의 실제 픽셀/animation timing이다. 새 기능 계약에 맞춰 선택적으로 추가한다. spritesheet texture+JSON 일괄 보호는 GAP-05/R4 잔여이며 이번 PNG sequence 검증과 혼동하지 않는다.

감색 animation/tilemap 및 반투명 alpha 보존, 범용 모드 전환·cel unlink 등 미노출 GAP 기능, GUI 지속 세션은 현재 지원 범위 밖이다. MCP 앱별 UI 수용/표시와 OS CI 자동화는 별도 잔여다. Docker bind mount `file_changed` 관찰의 원인은 이번 container 내부 filesystem 실행으로 해결됐다고 판단하지 않는다.

## 실행 기록

실행 환경은 Docker image `pixel-mcp-ci:latest` (`b0b99e1a1e34`), Linux amd64, `go version go1.25.14 linux/amd64`, CLI `Aseprite 1.3.18.3-dev`다. `go run ./cmd/pixel-mcp --health`는 success=true, API 41을 반환했다. 태그만 보고 정식 버전으로 기록하지 않는다.

검증은 전용 컨테이너 `pixel-regression-matrix`에 작업 사본을 복사하고 `/workspace`와 `/tmp`의 container 내부 filesystem에서 실행했다. 사용자 config 대신 `PIXEL_MCP_CONFIG=/tmp/matrix-config.json`을 사용했다(aseprite_path=`/build/aseprite/build/bin/aseprite`, temp_dir=`/tmp/pixel-mcp-matrix`, timeout=30). Linux 검사는 아래 순서로 실행했다.

```sh
go build ./...
go vet ./...
go test -count=1 -race -cover ./...
go test -count=1 -tags=integration ./...
```

- build / vet: 통과.
- unit·MCP 기본 테스트 race/coverage: 통과. pkg/aseprite 83.0%, pkg/tools 64.0%, pkg/server 90.7%. 이는 기본 suite의 statement coverage이며 matrix 완성률이 아니다.
- 신규 네 case 단독: Linux 6.684초 통과.
- macOS arm64: 설치된 Aseprite `1.3.18.2-arm64`. Docker Go 1.25.14로 `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -tags=integration -o /tmp/matrix-tools-darwin.test ./pkg/tools` 후 호스트에서 `-test.run TestRegressionMatrix -test.v` 실행, 네 case 7.01초 통과. RM-FIX-01의 거부도 동일하게 재현했다. 별도 config와 temp_dir을 사용했다. macOS 전체/race 실행을 뜻하지 않는다.
- PBT: 저장소에서 별도 property-testing/fuzz suite를 찾지 못했다. 이번에는 구체적 위험 fixture의 전 픽셀/bytes 불변식으로 검증했으며 별도 PBT 프레임워크 도입이나 무작위 전체 곱 생성은 하지 않았다.
- 지원 하한 재실행: 기존 전체 이력이 있고 릴리스 직전/명시 요청이 아니므로 NEXT_STEPS 정책에 따라 반복하지 않았다.
- 미실행: macOS 전체/race, 클라이언트 앱 UI, 위 잔여 교차 조합. 이 결과는 전체 matrix 완료 또는 배포 승인 근거가 아니다.

- Linux 전체 integration: 통과. `ok  	github.com/willibrandon/pixel-mcp/pkg/tools	298.676s`. 실패·skip을 임의로 제외한 필터 실행이 아니라 `./...` 전체 실행이다. 기본 suite 일부 환경 조건부 skip 여부를 별도 전수 감사했다는 뜻은 아니다.
- `gofmt -l` 신규 Go 파일 출력 없음, `git diff --check` 통과. 로그는 로컬 `/tmp/pixel-mcp-regression-matrix-evidence/{linux-race.log,linux-integration.log,macos.log}`에 보존했다(임시 경로이므로 영구 artifact는 아님).

## 공통 회귀 작업 완료 상태 (당시 기록; PR #27로 이후 병합)

회귀 작업 당시 테스트·본 보고서와 NEXT_STEPS/ROADMAP/CAPABILITIES 동기화를 포함했다. 해당 세션은 production/GAP 기능 구현, develop/main 병합, 배포를 수행하지 않았다. 이후 PR #27 develop 병합은 완료했다. 외부 독립 코드 리뷰는 아직 받지 않았다. 별도 RM-FIX-01 수정은 이후 PR #29로 develop에 병합했다. 회귀 문서·테스트는 PR #27로 develop에 통합했다. 아래 셀프리뷰/검증은 회귀 작업 당시 기록이며 신규 조회 검증과 구분한다.

셀프리뷰(`6507405..08ef4cb`): 병합 차단 P0/P1 없음, PASS_WITH_NOTES. SHARED-P3-001은 마지막 범위 밖 좌표 요청이 그룹 내부 레이어 조회 오류로 먼저 실패하여 좌표 검사 자체를 검증하지 못하는 보강 권고다. 후속 수정에서 요청을 루트 레이어 상태로 옮기고 `Pixel coordinates must be within sprite bounds`와 요청 직전/직후 bytes 동일성을 assert하도록 보강했다. 신규 matrix와 기존 RGB 좌표 거부 테스트 재실행은 6.607초 통과했다. 문서 경로·상태 동기화 권고는 이번 문서 변경에 반영했다. 이 기록은 후속 테스트 변경에 대한 재리뷰를 대신하지 않는다.

### 좌표 거부 검사 보강 및 재리뷰

- SHARED-P3-001: **VERIFIED**. 루트로 옮긴 paint에 범위 밖 좌표를 요청하고 좌표 오류 문구·요청 전후 원본 bytes를 검증한다. 그룹 조회 실패 재현과 분리했으며 이후 정상 drawing·native link·PNG 전체 픽셀 검사는 유지한다.
- Linux Go 1.25.14/Aseprite 1.3.18.3-dev: `go test -count=1 -tags=integration ./pkg/tools -run 'TestRegressionMatrix|TestIntegration_DrawPixels_RejectsCoordinatesOutsideSprite' -v` 통과, 6.459초. 신규 네 case와 기존 RGB 좌표 거부 검사를 실행했다.
- macOS arm64/Aseprite 1.3.18.2-arm64: 동일 필터로 재빌드한 테스트 바이너리 실행 통과(신규 matrix 6.81초, 기존 좌표 거부 0.34초). 로그는 기존 임시 evidence 디렉터리의 `linux-repair.log`, `macos-repair.log`에 보존했다.
- production 변경이 없는 테스트·문서 보강이므로 이전 전체 build/vet/race/integration 이력을 유지하고 관련 검사만 재실행했다. 전체 suite·macOS 전체/race·지원 하한은 반복하지 않았다.
- 리뷰 범위: 기준 `6507405`부터 최종 변경까지 테스트 1개·문서 4개. 직접 helper와 drawing handler/Lua를 문맥으로 확인했다. 별도 RM-FIX-01 구현, 다른 도구 확장, 배포는 제외했다.
- 이번 재리뷰 활동: 초기 범위/지적 확인 1회, 수정 cycle 1회, 수정 diff 검토 및 closure 1회(동일 pass), 전체 범위 fresh-discovery 1회, 총 review pass 3회. 최초 지적 P3 1건 → VERIFIED 1건. 신규 지적·수정 유발 결함·재개방·미해결 범위 내 지적은 0건. 최종 후보 선택 후 관련 변경/무효화 0회.
- 판정: **PASS_WITH_NOTES**. 남은 Notes는 별도 FIX인 RM-FIX-01(그룹 내부 편집 미지원 현상)과 기존 미검증 교차 조합·앱 UI·macOS 전체/race다. 범위 내 수정이 검증되었고 새 병합 차단 결함이 없어 추가 수정 cycle을 진행하지 않는다. 독립 외부 리뷰와 develop 병합은 수행하지 않았다.

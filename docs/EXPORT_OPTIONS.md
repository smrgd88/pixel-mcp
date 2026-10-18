# 범위 export 및 출력 보호 (GAP-05)

작업: `[BE][FEATURE] 범위 export 및 출력 보호`, `feature/be-export-options`.
최초 구현 기준: `origin/develop` `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732` (PR #29/#30/#31 병합). 현재 상태: PR #35로 2026-10-18 develop `99f193ad3a0eefcb8e8b71a3b9e3f69be47ac2d8`에 병합 완료 / Unreleased. 릴리스·plugin 배포 완료와 구분한다. 초기 독립 구현 당시 도구 추가 없이 **58개** 이름 집합을 유지했다. 병합 전 통합 검증에서는 PR #33/#34/#32가 병합된 `origin/develop` `7cc9b1d5166b2c1ec5e610e8419c3cce2b2f3ec7`을 통합하며 **64개** 등록 이름 집합을 유지했다. 병합 후에도 도구 수는 64개다. 미병합 브랜치를 직접 가져오지 않았다. 아래 초기 검증·리뷰 기록은 당시 이력이다.

## 호환성과 옵션

기존 필수 입력, 성공 응답 필드, `frame_number=0`의 전체 export, PNG/JPG/BMP의 `stem_0001.ext` 번호 체계는 유지한다. 단일 출력의 `exported_path/file_size`, 시퀀스의 `files[{path,file_size,frame_number}]`, sheet의 `spritesheet_path/frame_count/metadata_path` 의미도 동일하다. 선택 sheet의 frame_count는 선택된 프레임 수다.

| 옵션 | export_sprite PNG/JPG/BMP | export_sprite GIF | export_spritesheet |
| --- | --- | --- | --- |
| tag, frame_start/end | 지원 | 거부; 기존 frame_number만 지원 | 지원 |
| layer_id, include_hidden | 지원 | 거부 | 지원 |
| expected_revision | 지원 | 지원 | 지원 |
| trim | 투명 여백을 프레임별로 자름 | true 거부 | native cel trim; 선택 결과에 보이는 native background가 있으면 canvas 유지 |
| extrude | 입력 없음 | 입력 없음 | bool, native 1px edge extrusion |
| border/shape/inner_padding | 입력 없음 | 입력 없음 | 각각 정수 0–100 |
| overwrite | 지원 | 지원 | texture+JSON 모두 적용 |

Sheet texture 확장자는 `.png/.jpg/.jpeg/.gif/.bmp`다. 확장자 없는 경로·native 파일·JSON 자체를 texture로 쓰는 요청은 거부한다. PNG는 lossless RGBA/indexed 검증 대상이고 JPEG는 lossy, GIF/BMP는 해당 native encoder의 색/alpha 제약을 유지한다. GIF animation 구조·재생 방향·반복 횟수 확장은 포함하지 않는다.

- `tag`: 저장된 native tag의 **정확한 이름**. 빈 이름·없는 이름·중복 이름은 거부한다. CLI wildcard/경로/예약 문자열을 해석하지 않는다. tag의 inclusive from/to를 오름차순으로 한 번씩 export하며 reverse/pingpong을 재생 순서로 확장하지 않는다. GAP-03 조회/수정 도구가 필요하지 않다.
- `frame_start`, `frame_end`: 각각 생략/null은 1/마지막 frame, 명시적 값은 1–65535의 정수. 0·음수·역순·문서 범위 초과는 오류. `tag`, frame range, 양수 `frame_number`는 상호 배타적이다. `frame_number=0`은 새 범위 선택과 함께 쓸 수 있다.
- `layer_id`: [get_sprite_structure](SPRITE_STRUCTURE.md)의 형제 index 경로와 `expected_revision`을 함께 전달한다. 이름 필터는 받지 않으므로 동일 이름·슬래시·따옴표·줄바꿈으로 다른 레이어를 선택하지 않는다. group은 하위 전체를 선택하며 부모 blend/opacity를 유지한다. 없는 ID는 오류다.
- 기본 visibility는 저장된 target/조상의 hidden 상태를 모두 반영한다. `include_hidden=true`는 layer_id가 필요하며 target·선택 하위·조상만 임시로 보이게 한다. 선택 밖 sibling은 포함하지 않는다. locked는 읽기 export를 막지 않는다. GUI 선택 상태는 사용하지 않는다.
- `expected_revision`: optional SHA-256 내용 token. layer_id에는 필수이고 다른 선택에도 사용할 수 있다. 구조나 파일 bytes가 바뀌면 `file_changed`로 거부하므로 새 구조 조회가 필요하다.
- `trim=false`, `extrude=false`가 기본. Sheet의 선택 결과에 보이는 native background가 있으면 native의 모서리 색상 기준 trim을 끄고 canvas를 유지해 불투명 배경 픽셀을 보존한다. background가 숨겨졌거나 layer_id 선택에서 제외되면 기존 투명 여백 trim을 적용한다. 저장된 background 타입·이름·팔레트는 변경하지 않는다. 보이는 indexed background의 불투명 mask index가 PNG에서 투명해지지 않도록 단일 팔레트인 경우에만 메모리 문서를 RGB로 변환한다. 여러 팔레트는 native exporter가 이미 RGB로 렌더링하므로 변환하지 않아 linked frame별 색을 보존한다. 이 경우 texture는 RGBA이며 source의 indexed bytes는 그대로다. 기존 `padding` 값은 border/shape/inner 세 값의 기본값이다. 개별 padding의 생략/null은 fallback, **명시적 0은 0으로 덮어쓴다**. shape는 shape 사이 여백, border는 sheet 바깥 여백, inner는 frame 안 투명 inset이다. native frame bounds에는 inner가 포함되고 extrude rim은 포함되지 않는다. inner>0이면 바깥 edge가 투명하므로 extrusion도 투명할 수 있다.
- 새 선택 또는 trim을 사용한 결과가 모두 투명이면 오류다. 일부 빈 프레임은 생략하지 않고 duration/순서를 보존하며 trim 시 1×1 투명 출력으로 남긴다. 기존 옵션만 사용한 빈 canvas export는 계속 허용한다.
- `overwrite` 생략/null=true로 기존 교체 동작을 유지한다. false면 계획된 출력 중 하나라도 존재할 때 아무 파일도 발행하지 않는다. 시퀀스의 요청 base 및 목록 밖의 오래된 번호 파일은 삭제하거나 overwrite 대상으로 취급하지 않는다.

범위 3–4는 `stem_0003.png`, `stem_0004.png`와 source frame_number 3/4를 반환한다. 범위가 한 프레임이면 요청한 단일 경로에 저장하고 files를 생략한다. 단일 trim 이미지에는 별도 offset 응답을 추가하지 않으므로 재배치용 offsets가 필요하면 sheet JSON을 사용한다.

```json
{"sprite_path":"/absolute/walk.aseprite","output_path":"/absolute/walk.png","format":"png","frame_number":0,"frame_start":3,"frame_end":4,"trim":true,"overwrite":false}
```

## JSON 및 source 보존

기존 sheet는 include_json=false여도 native 명령에 dataFilename을 전달해 JSON sidecar를 생성했다. 이 기본 파일 동작을 유지한다. **include_json은 응답 metadata_path의 포함 여부**이며, 두 파일은 언제나 함께 계획·검증·보호한다.

Native JSON의 frame key와 기존 frame/rotated/trimmed/spriteSourceSize/sourceSize/duration, meta의 native 부가 필드를 유지한다. 각 frame에 1-based `source_frame_number`를 추가한다. `meta.image`는 반환된 metadata 경로와 같은 요청 폴더의 texture basename이며 private staging 경로를 노출하지 않는다. `meta.frameTags`는 선택 구간과 교차하는 범위만 남기고 출력의 0-based index로 옮긴다. direction/color 등은 보존한다. tag 방향은 metadata이며 실제 출력 순서는 source frame 오름차순이다. JSON 객체 key의 직렬화 순서는 계약이 아니다.

Aseprite 1.3.x sheet serializer가 이름/data의 quote와 backslash는 escape하지만 제어문자는 그대로 쓰는 동작을 실제 fixture와 native source에서 확인했다. 문자열 내부의 raw 제어문자만 JSON escape로 정규화한 뒤 strict decoder로 검증한다. 값은 바꾸지 않으며 잘못된 JSON 문법을 임의 복구하지 않는다. 등록 수·응답 payload와 달리 JSON sidecar의 추가 필드 및 tag index 보정은 plugin 소비자가 반영해야 한다.

source는 저장하지 않는다. native linked image, frame별 palette, 계층·위치·opacity·사용자 metadata를 원본 bytes 그대로 유지한다. visibility 변경은 개별 Lua process의 메모리 안 transaction에 한정한다. export는 자동 history 대상이 아니며 기존 edit의 undo와 수동 snapshot/restore를 바꾸지 않는다.

## 출력 보호와 경계

1. Sprite export는 source 잠금에서 범위·tag·크기·duration·revision을 계획하고 잠금을 해제한 뒤 source/base/실제 출력 전체를 정렬해 잠근다. Sheet는 출력 두 경로가 고정이므로 source와 함께 잠근다.
2. source revision을 계획 전후, 전체 잠금 획득 후 및 staging 생성 후 발행 전에 확인한다. PR #18의 count-only 재획득 검사보다 엄격해져, 프레임 수가 같아도 bytes가 달라졌으면 재시도를 요구한다.
3. [WithOutputFiles](FILE_PROTECTION.md)의 per-output staging·기존 백업·권한 보존·원본 변경 감지·fsync·atomic replace·오류 rollback을 재사용한다. source/texture/sidecar alias, 중복 출력, hard-link, read-only·디렉터리·부적절한 canonical 확장자를 거부한다. 부모 디렉터리가 없으면 기존 정책대로 만든다.
4. Sheet texture는 native decoder로 재열고 PNG/JPG/GIF는 Go decoder의 크기와 대조한다. JSON의 frame 수·duration·texture bounds·sourceSize·trim offsets·meta.size를 검사한 뒤에만 발행한다. 출력별 확장자는 요청한 형식과 맞아야 한다.
5. 오류/취소 시 이미 발행된 출력은 역순 복구한다. 복구 실패의 `file_rollback_failed/error.recovery`는 PR #18 및 OPS-03과 동일하며 sheet 순번 1은 texture, 2는 JSON이다.

Canvas/결과 한 변 32767 및 64 Mi pixels 상한을 유지한다. 아래 후보는 선택된 frame의 실제 trim bounds와 native 배치를 계산한다. Packed의 정밀 배치는 1.3.18.2 이상에 적용하며, 이전 지원 버전은 아래 보수 경로를 유지한다. 계산량 제한에 걸리는 복잡한 packed 입력은 frame 범위를 나눠 export한다. timeout은 30초 기본을 유지한다.

일반 오류 rollback은 **세트 전체의 crash 원자성**이 아니다. 협력하는 호출은 경로 잠금을 통해 완성된 세트를 관찰하지만 외부 reader는 교체 도중 일부 새 파일을 볼 수 있다. 비협력 writer의 ABA/최종 검사 이후 경쟁, 강제 종료·전원 장애 자동 복구 및 고아 staging 정리는 후속이다. Source history는 출력 세트 undo가 아니다. GAP-06 slice/pivot/nine-slice 및 편집 도구는 이번 범위 밖이며 기존 native slice metadata를 확장/재해석하지 않는다.

오류 코드는 [ERRORS](ERRORS.md)를 따른다. Go 입력/예산/overwrite 검사는 invalid_arguments, stale source는 file_changed, 기존 frame_number 범위 오류는 invalid_arguments를 유지하고, 새 native tag/layer/range/blank 검사는 lua_error, 파일·cancel·rollback은 기존 typed 분류다. 새 공개 오류 코드를 추가하지 않는다.

## 후속 작업 추적

Background/출력 형식의 plugin 안내는 plugin PR #16에서 반영됐다. Indexed PNG 유지 옵션 설계, 현재 후보의 크기 예산 정밀화, 출력 세트 crash 복구 설계는 [NEXT_STEPS의 export 후속 TODO](NEXT_STEPS.md#export-후속-todo-pr-35-범위-밖)에 별도 작업명과 완료 조건으로 등록했다. **PR #35는 이 후속 기능을 구현하지 않는다.** 현재 계약·검증 결과와 후속 작업의 완료 여부를 구분한다.

**명시적인 남은 작업:** 구버전 Aseprite의 정밀 packed 계산, Aseprite 프로세스 전체 메모리 제한, 출력 세트 crash 복구, 비협력 writer 경쟁 보호는 미구현이다. [작업별 후속 범위와 완료 조건](NEXT_STEPS.md#크기-예산-pr-이후-남은-작업)에 등록했다. 현재 구현의 관련 검증은 완료했으며, 이 네 기능을 구현하거나 그 기능의 검증까지 완료한 것은 아니다.

## 검증과 plugin 후속

테스트: `TestExportOptions*`는 실제 MCP/Aseprite 저장·재열기, RGB/grayscale/indexed mask=3와 opaque index 0, native linked/positioned cels, frame palette 변화, 그룹/숨김/중복 이름, tag/range/빈 frame, stale/invalid/aliases/overwrite/폴더/read-only/cancel/source 변화, 독립 PNG 전체 픽셀·JSON bounds/duration/trim/extrude 및 history/snapshot/undo를 검사한다. 순서/번호 및 JSON escape는 deterministic testing/quick 각 1000개 입력으로 검사한다. 기존 `TestExportSequence*`, `TestOutputFiles*`/`TestOutputRollback*`와 server diagnostics/등록 검사를 함께 수행한다. 실제 실행 결과는 아래 최종 기록으로 구분한다.

Plugin 저장소·MCP pin·번들은 PR #35에서 수정하지 않았다. 이후 [plugin PR #16](https://github.com/smrgd88/pixel-plugin/pull/16)이 develop `f5228e3`로 병합돼 아래 항목을 MCP `99f193a` / 64 tools 기준으로 반영했다. MCP 문서 PR #36의 `42bab18`은 runtime pin이 아니다. 개인 설치본 교체·릴리스와 구분한다. 아래 목록은 PR #16의 반영 범위다:

- 기존 export_sprite/export_spritesheet의 optional 입력 schema와 서버 commit을 반영했다. 도구 이름/수 추가는 없다.
- exporter skill에 `get_sprite_structure → layer_id+revision`, tag/range 배타 규칙, hidden/blank 정책, padding override, overwrite=false, 범위의 원본 번호 규칙을 추가했다.
- sidecar source_frame_number와 출력 기준 frameTags, include_json=false의 legacy sidecar, texture+JSON rollback 및 recovery 순번을 소비자/스킬에 반영했다.
- crash 원자성이나 출력 history undo를 주장하지 않는다. GIF 새 선택은 지원하지 않는다.

PR #33/#34/#32 develop 통합 시 공통 문서 충돌을 양쪽 계약과 검증 이력을 보존하며 해소했다. animation/canvas/server 등록은 세 기능을 유지하며 export 자체의 도구 추가는 없다. PR #35도 develop `99f193a`에 병합 완료했다. 아래 실행·리뷰 기록의 미병합 표시는 당시 후보 상태를 뜻한다.

근거: [ExportSpriteSheet API](https://www.aseprite.org/api/command/ExportSpriteSheet/), [Image API](https://www.aseprite.org/api/image/), Aseprite 1.3.18.3 native `cmd_export_sprite_sheet.cpp`, `layer_frame_comboboxes.cpp`, `doc_exporter.cpp`. 공식 API의 존재와 이 후보의 실제 검증 범위를 구분한다.

## 크기 예산 정밀화 구현 후보

작업: `[BE][FEATURE] export 크기 예산 정밀화 및 MCP 문서 현황 갱신`, 기준 `origin/develop@42bab1833a15c9bb6bd50990d4e6102af3d8fa1b`. 현재 작업 브랜치 후보이며 develop 미병합/Unreleased다. Plugin 저장소·설치본·번들을 수정하지 않는다.

원인과 작은 재현: 128×128의 서로 다른 65프레임은 rows에서 8320×128, columns에서 128×8320이다. 기존 검사는 두 strip extents를 곱한 8320×8320을 사용해 64 Mi pixels를 초과한다고 거부했다. 같은 fixture의 정상 packed도 거부했다. 또한 32767×1 canvas의 1px 내용 두 프레임은 trim 후 2px sheet가 되지만 기존 검사는 원래 canvas strip으로 거부했다. 상한을 올리거나 제거하지 않는다.

계획과 실행:

1. 기존 source 잠금과 revision 검사 안에서 tag/range/layer/group/visibility를 적용한다. 한 변 32767, canvas 64 Mi pixels, frame endpoint 1–65535를 **곱셈·차 연산 전에** 검사한다.
2. Sheet bitmap 대신 한 프레임 크기의 scratch image로 native source color mode의 렌더·trim bounds를 측정한다. 빈 trim frame은 1×1, 보이는 native background는 canvas 유지, 단일 팔레트 indexed background는 메모리 RGB 준비를 공유한다. 다중 palette 문서는 변환하지 않는다. Lua userdata가 native 이미지 메모리를 오래 잡지 않도록 매 프레임 수집한다. 비-trim strip은 scratch render가 필요 없다.
3. Native `DocExporter`와 LAF `PackingRects`의 배치 규칙을 따른다. horizontal/vertical은 strip, rows/columns도 현재 public 옵션처럼 행·열 수/고정 크기를 지정하지 않으면 각각 같은 strip이다. 1.3.18.2 이상에서 Packed는 source-mode 렌더가 같은 sample을 합치며(네이티브는 `mergeDuplicates=false`에도 적용), 면적 내림차순의 안정 정렬과 첫 빈 위치 배치, 가로/세로 후보 크기 교대 증가를 사용한다. 회전은 하지 않는다. Border/shape/inner와 extrusion을 포함해 계산하며 JSON bounds는 inner를 포함하고 extrusion rim을 제외한다. 임의의 grid 알고리즘으로 native와 같다고 가정하지 않는다.
4. 한 변·최종 bounding rectangle 64 Mi pixels 외에 packed의 고유 sample 면적 64 Mi pixels, geometry work 8,000,000회, 계획 계산 30초 및 요청 context deadline/cancel을 적용한다. Native signed-int candidate area의 overflow도 차단한다. 이는 프로세스 전체 RSS 제한이 아니다: source decode와 native renderer의 내부 임시 메모리는 별도이며 scratch/고유 sample 저장은 유한하게 제한한다. 복잡한 packing은 결과가 작아도 계획 예산으로 거부될 수 있다.
5. Native 출력 직전 다시 측정해 sample bounds·중복 관계를 비교한다. 계획 후 source가 바뀌어 더 큰 native 할당을 유발하지 않게 한다. 실제 texture 재열기/독립 decoder 크기와 JSON 각 frame placement·trim·duration·sourceSize를 계획과 대조하고 revision을 재검사한 후 기존 texture+JSON staging/rollback으로 발행한다. 불일치는 발행하지 않으며 source는 저장하지 않는다.

**지원 하한의 packed 제한:** 1.3.17.2 소스의 LAF는 `std::sort`, 1.3.18.2는 `std::stable_sort`를 사용한다. 같은 면적의 서로 다른 모양은 표준 라이브러리에 따라 순서와 최종 크기가 달라질 수 있다. 따라서 **1.3.18.2 미만의 supported runtime에서는 packed만** 실제 trim/중복을 반영한 가로·세로 strip envelope를 먼저 제한하고 native 배치를 그대로 실행한다. 이 경로는 결과 크기를 envelope 이하로 검사하고 각 frame의 크기·trim·duration/source를 대조하지만 XY 배치를 최신 버전과 동일하다고 강제하지 않는다. 예전 지원 요청을 보존하며 일부 정상 packing의 과잉 거부는 남는다. 65×128² packed 사례의 정밀 허용을 이 버전들까지 완료했다고 주장하지 않는다. 나머지 네 layout의 trim 기반 정밀화는 동일하게 적용한다. 정밀화를 구버전 packed까지 확장하려면 bitmap 생성 전 native placement API 또는 별도의 표준 라이브러리 차이를 다루는 설계가 필요하다.

공식 [ExportSpriteSheet API](https://aseprite.org/api/command/ExportSpriteSheet)와 [v1.3.18.2 DocExporter](https://github.com/aseprite/aseprite/blob/v1.3.18.2/src/app/doc_exporter.cpp), 해당 tag의 [LAF PackingRects](https://github.com/aseprite/laf/blob/68259e1904fab548529e78583af71869fca1cb53/gfx/packing_rects.cpp)를 대조했다. Linux의 1.3.18.3-dev 소스도 같은 규칙이다. 지원 하한의 [LAF 소스](https://github.com/aseprite/laf/blob/aee8822156619444c08718322ff5d1b18aab2a5b/gfx/packing_rects.cpp)와 정렬 차이도 대조했다. JSON-only native export 역시 bitmap을 먼저 할당하므로 preflight로 사용하지 않는다. `Image.drawSprite`의 process-local compose_groups 설정은 native 기본 false로 측정하는 동안만 적용하고 성공/실패 모두 원래 값으로 복원한다. 사용자 설정 파일을 변경하지 않는다.

`TestExportBudget*`는 과거 거부 재현, 실제 native exporter를 별도 oracle로 삼은 3모드×5layout×trim on/off의 출력 크기/JSON·픽셀·빈 frame·서로 다른 크기, wide-canvas trim의 4형식×5layout, 렌더 직전 변경 거부를 검사한다. 별도 occupancy-grid oracle의 결정적 500개 입력으로 최적화된 packing 탐색을 대조한다. 상한/64-bit overflow/과도 frame/padding/cancel은 큰 bitmap 없이 계획 단위에서 검증한다. 기존 background·palette·native link·선택/hidden/group/tag·overwrite/alias·취소/commit failure·history 회귀도 함께 실행한다. 실행 결과와 셀프리뷰는 아래 최신 검증 기록에 남긴다.

제한: 구버전 packed의 보수 경로가 남으며 native 배치 구현에 종속된 계획이다. 이후 Aseprite의 알고리즘 변경은 새 differential 검증과 계획 갱신이 필요하다. 출력 대조는 불일치의 발행을 막지만 미래 버전의 다른 내부 할당 전략까지 증명하지 않는다. 기존 crash 원자성/비협력 writer 경쟁 한계와 모든 format×blend 조합 미검증을 유지한다. Plugin 후속은 새 MCP pin 채택 시 크기 예산/계산량 제한·rows/columns/packed 설명을 갱신하는 것이며 이번 작업에서 번들을 재빌드하지 않는다.

## 2026-10-04 실행 결과와 셀프리뷰

- Linux amd64: 전용 `pixel-export-options` 컨테이너, image `pixel-mcp-ci:latest`, Go 1.25.14, 실제 Aseprite `1.3.18.3-dev` / API 41. `/tmp/export-options-config.json`, 별도 temp, timeout 30초, GOMAXPROCS=2 / `-p 1`. container 내부 filesystem에 소스를 복사해 build → vet → health → 전체 race/coverage → 전체 integration 순서로 실행했다.
- 전체 build/vet/health/race/coverage/integration 통과. coverage: pkg/aseprite 82.3%, pkg/tools 64.2%, pkg/server 90.7%. 전체 integration의 pkg/tools는 370.543초였다. statement coverage이며 조합 완성률이 아니다.
- 전체 검사 시작 후 아래 리뷰 수정과 test-only 경계값 보강이 있었다. 최종 후보는 다시 build/vet하고 `go test -p 1 -count=1 -race -tags=integration ./pkg/aseprite ./pkg/tools ./pkg/server -run 'TestExportOptions|TestExportSequence|TestExportSpritesheet|TestRegressionMatrix|TestOutputFiles|TestOutputRollback|TestFileProtectionSheet|TestToolDiagnostics|TestSnapshotToolsRegistered' -v`를 실행해 통과했다. aseprite 1.213초, tools 30.359초, server 4.170초. 전체 suite 재실행과 최종 영향 범위 검증을 구분한다.
- macOS arm64: 로컬 Go 1.25.0, Aseprite `1.3.18.2-arm64` / API 41, 별도 config/temp. 최종 CLI build/health와 같은 필터의 integration(core 출력 보호·MCP export/오류·등록·matrix)을 순차 실행해 통과했다. aseprite 0.763초, tools 32.985초, server 2.950초. macOS 전체/race 검증은 아니다.
- Sequence PR #18 및 OPS-03의 파일 목록/번호/실패·rollback 복구 정보/공개 오류/request ID 검사를 재실행했다. texture 생성 후 취소/외부 source 변경은 실제 native 결과를 staging에 생성한 뒤 검사하며, publication 실패/복구 실패는 재사용하는 output-files core의 실제 파일 교체 실패 주입으로 검사한다. 가짜 Aseprite 실행 파일을 사용하지 않았다.
- 도구 이름 집합은 기준·후보 모두 58개로 동일하다. 실제 server 등록 수와 기존 필수 입력·성공 응답 shape, optional/null/명시적 0, source revision 및 bytes/inode/mode/mtime, metadata/native link/프레임별 palette, 독립 PNG 픽셀 검사를 확인했다. 두 deterministic property test는 각각 1000개 입력으로 통과했다.
- `gofmt -l`, `git diff --check`, 변경 문서의 로컬 상대 링크 검사 통과. Linux 실행 사본과 최종 source 해시를 확인했다. 마지막 관련 검사에 skip/failure 출력은 없었다. 전체 기본 suite의 환경 조건부 skip을 별도 전수 감사했다는 뜻은 아니다.
- 미실행: macOS 전체/race 및 모든 format×mode×계층 조합의 전수검사. 지원 하한 전체 suite는 기존 이력과 개발 정책에 따라 반복하지 않았다. GAP-06 및 GUI/새 GIF 구조는 구현 범위 밖이다.
- 초기 restricted sandbox에서 Docker/git index/network 접근 실패와 Aseprite version probe exit 134가 있었다. 사용자 제공 시스템 로그는 application 등록 전 WindowServer/LaunchServices mach-lookup 거부를 가리킨다. 이는 기능 결함 기록이 아니다. 사용자 승인으로 동일 세션 환경을 복구한 뒤 위 검사를 실제 재실행했다. 초기 순수 config unit 통과와 이 최종 결과를 구분한다. 초기 동시 macOS 작업 중 capability timeout 1회도 있었고 timeout을 늘리지 않은 순차 재검사는 통과했다.
- 개발 중 newline metadata의 native JSON 실패를 위 정규화로 수정했고, indexed 완전 투명 픽셀 비교는 의미 없는 RGB 대신 premultiplied RGBA 기준으로 검증했다. 이 개발 단계의 실패와 아래 후보 리뷰 finding은 구분한다.
- 로컬 로그/manifest는 `/tmp/pixel-mcp-export-options-evidence/`의 `linux-full.log`, `linux-final.log`, `macos-final.log`, `candidate*.manifest`에 보존했다. 임시 검증 증거이며 배포 artifact는 아니다.

`convergent-code-review` backend lens, review-and-repair 모드. 기준/initial HEAD/merge-base는 모두 `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732`. 범위는 변경 21개 파일(Go 8, 문서 13)이다.

| ID | 우선순위·상태 | 원인·수정·검증 |
| --- | --- | --- |
| BE-P1-001 | P1 · VERIFIED | overwrite=false의 최초 존재 확인과 output staging 사이에 비협력 writer가 파일을 만들면 그 bytes가 새 baseline으로 수용돼 덮어쓸 수 있었다. 공통 export 출력 wrapper에서 staging snapshot/backup 이후 조건을 다시 검사한다. `TestExportOptionsOverwriteRecheckedAfterStaging`은 이 간격에 생성된 sidecar의 bytes 보존, texture 미생성 및 staging 정리를 검사한다. overwrite=true 대조도 통과 |
| BE-P2-002 | P2 · VERIFIED | repair diff의 기존 계약 대조 중 legacy frame_number 초과가 기존 Go invalid_arguments에서 Lua 오류로 바뀐 것을 확인했다. 기존 양수 frame_number는 먼저 전체 count를 계획하고 Go에서 범위를 검사하도록 복원했다. 실제 제품 서버 `TestExportOptionsDiagnostics`에서 invalid_arguments 유지 및 새 range/tag의 lua_error, stale file_changed, request ID/redaction/성공 shape를 검증 |

초기 discovery 1회, repair cycle 1회(위 두 원인의 bounded 수정), repair-diff 1회, closure 1회, fresh-discovery 전체 범위 1회, 총 review pass 4회. 초기 P1 1건, repair 중 P2 1건 추가; closure/fresh 신규 finding 0건. P0·repair 유발 결함·재개방·accepted/out-of-scope finding·미해결 finding 0건. 차단 위험은 초기 P1 1 → repair/closure/fresh 0이다. 두 finding은 모두 검증됐으며 마지막 변경 이후 전체 범위에서 새 차단 원인을 찾지 못해 mutation을 종료했다. 전체 소요 시간은 별도 측정하지 않았다.

- Reviewed: export schema/handler/planner/generator/metadata/output precondition, 관련 새 회귀와 한·영 inventory/계약/현황 문서.
- Inspected for context: 기존 Client, file protection 및 output-files locking/backup/rollback, structure/cel revision, history/snapshot wrapper, diagnostics, native export 구현 및 기존 fixture/PR #18 회귀.
- Changed during repair: export_sprite/options/spritesheet, overwrite unit 회귀, 별도 server diagnostics 회귀; schema 설명과 현재 상태/오류 계약 문서도 대조했다. 공통 보호 core 자체는 수정하지 않았다.
- Excluded: 다른 Top4 구현/브랜치, plugin, 새 등록 도구, GAP-06, GIF 구조, 공통 저장소/권한 설정, 병합·태그·배포.

최종 후보(보고 절 추가 전) manifest SHA-256은 `a0001147a4ed79793dd231d9b8fc5bffbd714a401a7ad59b842595f107adb709`, Go 8개 파일 manifest는 `0826dd599ebaa4be5345dbb68486d82c0b446b2f642c13d971e5b0c4855681df`다. 최종 후보 선정 이후 runtime/test/계약 mutation 및 인증 무효화는 0회이며 이후 추가는 이 실행·리뷰 보고다.

Notes:

1. 출력 세트는 일반 오류 rollback을 제공하며 crash 원자성/외부 reader의 중간 상태 차단/비협력 writer의 최종 검사 이후 경쟁을 보장하지 않는다.
2. 보수적 packing 예산은 실제 들어갈 큰 sheet도 거부할 수 있다. 위 미실행 범위를 전체 matrix 완료로 해석하지 않는다.
3. Plugin pin/번들/exporter skill 동기화는 이 PR 병합 뒤 별도 작업이다. 독립 외부 리뷰와 develop/main 병합·배포는 수행하지 않는다.

판정: **PASS_WITH_NOTES**.

## 재리뷰 지적 수정 (e0ef590 이후)

작업: `[BE][FIX] export 리뷰 지적 수정`. 기준/merge-base `96d8fa4`, 수정 시작 HEAD `e0ef590`. PR #35의 해당 HEAD CI가 통과했지만, 별도 read-only 재리뷰에서 다음 두 경로를 실제 MCP로 재현하여 BLOCKED로 판정했다. 이 기록은 앞선 후보의 PASS_WITH_NOTES를 대체한다.

| ID | 우선순위·최종 상태 | 재현·원인·수정·검증 |
| --- | --- | --- |
| BE-P1-003 | P1 · VERIFIED | 16×16 native background의 파란 불투명 250px + 빨간 6px를 sheet trim이 2×3 빨강만 남기고 성공 반환했다. 같은 source의 export_sprite trim은 16×16이었다. Native exporter가 background의 corner color를 trim 기준으로 쓰기 때문이다. 선택 후 보이는 background가 있으면 sheet trim을 끄고 canvas를 유지한다. RGB/grayscale 및 indexed mask 0/3의 전체 픽셀·JSON offsets, locked background/명시적 ID 선택, background를 제외한 foreground trim을 검증 |
| BE-P2-004 | P2 · VERIFIED | 1×32767 horizontal/32767×1 vertical에 padding을 더하면 짧은 축 검사가 생략돼 native export 후 operation_failed로 실패했다. padding/extrusion/border를 합친 두 축을 모두 사전 검사한다. 각 방향 × border/inner/extrude 실제 handler 검사에서 invalid_arguments와 출력 부모 디렉터리 미생성을 확인 |
| BE-P1-005 | P1 · VERIFIED | cycle 1에서 canvas를 보존한 뒤, native indexed sheet가 background의 불투명 mask index를 투명하게 저장하는 별도 기존 문제를 확인했다. source의 단일 팔레트·보이는 background 조합만 **메모리에서** RGB로 변환한다. native가 이미 RGB로 출력하는 다중 팔레트 문서는 변환하지 않아 linked frame별 색이 합쳐지지 않게 한다. 단일/프레임별 palette × linked background × trim on/off의 PNG 픽셀·extruded rim·duration, 원본 bytes/native mode/link 보존 검증 통과 |

- 수정 전 정식 회귀에서 grayscale/indexed의 2×3 축소와 두 배치의 operation_failed를 확인했다. RGB는 직전 실제 MCP probe에서 250px 손실을 확인했으며 정식 red 첫 실행에서는 별도 capability timeout이 발생했다. timeout을 늘리지 않았고 후속 green/최종 회귀는 통과했다.
- cycle 1: background trim 및 양축 검사를 수정했다. RGB/grayscale 보존과 P2는 검증됐고, 새 indexed mask 문제(BE-P1-005)가 남았다. 이는 수정으로 새로 만든 결함이 아니라 같은 native 출력 경로에서 추가로 드러난 문제다.
- cycle 2: 단일 팔레트 indexed background의 RGB 준비를 추가했다. 기존 frame-palette 회귀와 새 linked background의 단일/다중 palette 대조를 통과했다. 더 이상의 mutation 없이 closure 및 전체 fresh-discovery를 수행했다.
- Linux amd64 Docker / Go 1.25.14 / Aseprite 1.3.18.3-dev: 전용 기존 컨테이너, config/temp 및 timeout=30, GOMAXPROCS=2/-p 1 유지. 최종 build·vet 및 앞 절과 같은 필터의 `-race -tags=integration` 검사 통과: aseprite 1.200초, tools 39.092초, server 3.924초. 출력 rollback/기존 sequence/공개 오류/등록 수와 native matrix 포함.
- macOS arm64 / Go 1.25.0 / Aseprite 1.3.18.2-arm64: 최종 build·health 및 동일 관련 integration 통과. 실제 실행 시간은 로컬 `macos-repair-final.log`에 기록했다. 필터에는 새 Background/PaddedAxes 테스트가 포함된다.
- 로컬 전체 suite/전체 coverage와 지원 하한 반복은 이 좁은 수정에서 재실행하지 않았다. 앞선 전체 결과·수정 전 CI와 최종 영향 범위 검증을 구분한다. 수정 commit의 CI는 push 후 별도 확인한다. macOS 전체/race 및 모든 형식×blend×mode 조합의 전수검사는 미실행이다.
- 로그: `/tmp/pixel-export-review35/{red.log,green.log,green2.log,linked-green.log,linux-repair-final.log,macos-repair-final.log}`. 수정 전 실제 MCP probe·독립 PNG decoder 결과도 같은 임시 폴더에 보존했다. 기본 설정·다른 작업·plugin은 수정하지 않았다.

`convergent-code-review` review-and-repair: 초기 지적 재확인 1회, repair cycle 2회, repair-diff 검토 2회, closure 1회, fresh-discovery 전체 1회, 총 review pass 5회. 초기 P1 1/P2 1, cycle 1 후 새 P1 1, cycle 2/closure/fresh 새 finding 0. 최종 VERIFIED 3, OPEN/REOPENED/ACCEPTED/OUT_OF_SCOPE 0. 이전 BE-P1-001/BE-P2-002는 재개방되지 않았다. 차단 P1은 초기 1 → cycle 1 잔여 1(원래 원인 해소·새 원인 확인) → cycle 2/closure/fresh 0으로 줄었다. 두 번째 cycle까지 실제 위험 감소를 검증했으며 새 차단 원인이 없어 종료했다. 전체 소요 시간은 측정하지 않았다.

Reviewed: 기준부터 최종 후보까지 22개 파일(Go 9, 문서 13), export 계약·계획/렌더/메타데이터/출력 보호·회귀·문서. Context: 기존 lock/staging/rollback, native Render/DocExporter/SetPixelFormat 및 기존 fixture. Changed during repair: generator, 차원 검사, schema 설명, 새 background/경계 회귀, 본 계약과 CHANGELOG(6개 파일). Excluded: 다른 Top4, plugin·등록 도구 확장, GAP-06/GIF 구조, 병합/태그/배포.

최종 후보(이 보고 추가 전) 22개 파일 manifest SHA-256: `6199d00dfec8ca78c46312dd6e510430a0808933a23f27ff59a871b01a20a4a5`. 이후 runtime/test/계약 변경과 인증 무효화는 0회다. 새 schema의 background 예외와 indexed background RGBA 출력 영향도 계약에 반영했다.

Notes:

1. 선택 결과에 보이는 native background가 있으면 canvas를 유지한다. foreground만 trim하려면 background를 제외하는 layer_id 선택을 사용한다. 단일 팔레트 indexed background texture는 RGB로 출력될 수 있으나 저장 source의 모드·팔레트·링크는 바뀌지 않는다.
2. 기존 보수적 크기 예산, 비협력 writer 경쟁, 일반 오류 rollback과 crash 원자성의 차이, 위 미실행 범위는 유지한다.
3. plugin pin/번들/exporter skill 동기화는 별도 후속이며, 이 background/출력 모드 계약도 함께 반영해야 한다. 병합·태그·배포는 수행하지 않는다.

수정 후 판정: **PASS_WITH_NOTES**. 직전 read-only 재리뷰의 BLOCKED를 대체한다.

## 최신 develop 통합 (2026-10-18)

작업: `[BE][CHORE] export 통합 및 다음 작업 최신화`. 시작 HEAD `6f466f8d360a90b170c975332cece12d85e9c12f`에 `origin/develop` `7cc9b1d5166b2c1ec5e610e8419c3cce2b2f3ec7`을 통합했다. PR #33 cel unlink, #34 layer/group, #32 tags는 develop 병합 완료이며 이 통합 검증 당시 PR #35 자체는 미병합/Unreleased였다(이후 병합 상태는 문서 상단 참조). 원래 develop 워크트리나 다른 작업 브랜치를 수정하지 않았다.

- 공통 문서 11개의 충돌을 해소했다. 최신 세 기능의 production/schema/tests는 develop과 byte-identical하게 유지했고, export production 5개 파일은 리뷰 완료본 `6f466f8`과 byte-identical하다. 배경 canvas/불투명 indexed mask, padding 후 두 축 검사, revision/output 보호를 되돌리지 않았다.
- 등록 이름 합집합은 기존 export 후보 58 + 최신 develop 64 = **64개**다. 실제 server list 전체 이름/개수 검사, CAPABILITIES MCP-001…064의 ID·이름 일치, README 한/영 inventory를 대조했다. 새 등록 도구는 없다.
- `TestExportIntegratedLayerTagUnlinkAndUndo` 추가: 세 모드에서 그룹 삽입·같은 이름의 레이어 이동으로 옛 ID가 다른 대상을 가리키게 한 뒤 stale export를 거부한다. unlink → 독립 frame의 x=0 수정 → tag rename/range/direction 수정 → 숨김 layer의 PNG/sheet export를 검사한다. 독립 PNG decoder의 전체 픽셀, JSON source frame/trim offset/duration/tag 범위, 오류 시 기존 texture+JSON/source 불변, export history 제외, 7개 편집의 역순 byte-exact undo 및 복원 revision 재사용을 확인한다.
- NEXT_STEPS/ROADMAP은 PR #35 통합 검증·승인된 병합 판단 → plugin 동기화 → GAP-04/GAP-06 별도 범위 선정으로 현황을 갱신했다. 기존 태그 후속 TODO 세 항목은 최신 develop과 문자 단위로 동일하게 보존했고, SAFE/OPS·전체 matrix·crash/외부 writer 제한과 과거 검증 기록을 소급 완료하지 않았다.

검증은 동시 native workload를 피하도록 순차 실행했다. Docker daemon이 사용 가능했으므로 CI fallback만으로 대체하지 않았다.

- Linux amd64 / 전용 `pixel-export-options` 컨테이너 / Go 1.25.14 / Aseprite `1.3.18.3-dev`, API 41: container 내부 source 사본, `/tmp/export-integration-config.json`, 전용 temp, timeout=30, GOMAXPROCS=2/-p 1. build·vet·health·전체 unit/race/coverage 통과. coverage는 aseprite 81.8%, tools 62.9%, server 90.7%이며 statement coverage다.
- Linux 최종 관련 `-race -tags=integration` 통과: aseprite 1.768초, tools 253.918초, server 9.235초. 필터는 `TestExport|TestTags|TestTagProperties|TestLayer|TestMoveLayer|TestCreateLayerGroup|TestUnlinkCel|TestCelProperties|TestSpriteStructure|TestStructureInput|TestRegressionMatrix|TestOutputFiles|TestOutputRollback|TestFileProtection|TestHistory|TestSnapshot|TestToolDiagnostics|TestIntegration_LinkCel`이다. 새 교차 회귀 외에 source/staging/lock/publication rollback/취소/metadata·background·frame palette 및 cel/link/unlink/tag/layer 단독 회귀를 포함한다.
- macOS arm64 / Go 1.25.0 / Aseprite `1.3.18.2-arm64`, API 41: 별도 config/temp에서 build·health 및 export 전체 관련 회귀, layer/cel/unlink/tag 저장·undo와 교차 회귀, structure·출력 rollback·history/snapshot·diagnostics·64개 등록 검사를 통과했다. 전체 macOS suite/race는 아니다. 상세 command와 소요 시간은 로컬 로그에 남겼다.
- 이번 로컬 Linux 전체 integration은 반복하지 않고 위 영향 범위를 실행했다. 원격 최종 HEAD CI의 전체 unit/race/integration은 push 후 별도 확인한다. 지원 하한 반복과 기존 범위 밖 조합은 재실행하지 않았다. 최종 관련 로그에 FAIL/SKIP은 없었다. 로그는 `/tmp/pixel-export-integrate-20261018/{linux.log,macos.log}`에 보존했다.
- 충돌 marker 없음, gofmt/diff 공백 및 변경 문서 상대 파일 링크 검사 통과. 현재 대화의 기본 cwd에는 정리된 cel-unlink 경로가 남아 훅 실행 시 OS error 2가 관찰됐지만, 모든 작업 명령은 실제 export 경로를 명시해 실행했다. 글로벌 훅/접근 설정이나 삭제된 워크트리를 복구·변경하지 않았다. 이 환경 이슈와 코드/검증 실패를 구분한다.

`convergent-code-review`: 통합 diff 41개 파일과 최신 develop 대비 PR 전체 24개 파일(Go 11, 문서 13)을 검토했다. 통합 diff 검토 1회, 최종 후보의 전체 fresh-discovery 1회, 총 review pass 2회. 통합/충돌 해소 1회, finding repair cycle·repair-diff·closure 0회(새 결함 수정 없음), 초기/신규/재개방/미해결 finding 모두 0건. 기존 리뷰 지적은 재개방되지 않았다. 범위는 export 및 등록/교차 회귀·문서이며, 새 develop의 layer/tag/unlink producer와 shared lock/history/structure는 직접 연동 문맥으로 대조했다. plugin·추가 기능·병합/배포는 제외했다.

최종 후보 manifest(보고 추가 전) SHA-256은 `e8fa9823bae847a67f59b0b48770a916f285e81a9d06f7a88fcc1f1c4d04bd3b`다. 최종 후보 선정 이후 runtime/test/계약 변경·인증 무효화 0회이며 이후 추가는 이 검증 보고다. 통합으로 이전 `6f466f8` 후보를 대체했으며 이전 후보의 테스트만으로 승인하지 않았다.

당시 Notes: 배경 유지/일부 PNG의 RGB 출력, 보수적 크기 예산과 crash/외부 writer 한계, 위 미실행 범위는 유지했다. 당시 plugin 동기화와 PR #35의 develop 병합은 별도 후속이었다. 로컬 판정은 **PASS_WITH_NOTES**였으며 최종 원격 HEAD CI를 확인한 뒤 보고하는 단계였다. 이후 CI 통과·병합 상태는 문서 상단과 NEXT_STEPS의 현재 상태를 따른다.

## 크기 예산 최종 검증과 셀프리뷰 (2026-10-18)

기준/initial HEAD/merge-base: `42bab1833a15c9bb6bd50990d4e6102af3d8fa1b`. 작업 경로 `/Users/keumheesung/orca/workspaces/pixel-mcp/be-feature-export-size-budget`, 브랜치 `feature/be-export-size-budget`. 아래 결과는 이 후보의 로컬 검증이며 develop 병합·릴리스·plugin 설치본 갱신을 뜻하지 않는다.

- Linux amd64 / 전용 `pixel-export-budget` 컨테이너 / Go 1.25.14 / Aseprite `1.3.18.3-dev`, API 41: 최종 build·vet·health, `go test -p 1 -count=1 -race -cover ./...` 통과. Statement coverage는 aseprite 81.3%, tools 64.6%, server 90.7%다. 전체 조합 완성률이 아니다.
- 최종 Linux 관련 integration/race: `go test -p 1 -count=1 -race -tags=integration ./pkg/aseprite ./pkg/tools ./pkg/server -run 'TestExport|TestOutputFiles|TestOutputRollback|TestFileProtection|TestToolDiagnostics|TestSnapshotToolsRegistered|TestRegressionMatrix' -v` 통과. aseprite 1.476초, tools 120.892초, server 4.592초. 실제 PNG/native metadata oracle, source bytes/readonly/alias, selection·background·palette/link, cancellation·commit/rollback/recovery, 등록 64개와 diagnostics를 포함한다.
- macOS arm64 / Go 1.25.0 / Aseprite `1.3.18.2-arm64`, API 41: 최종 build·vet·health 및 같은 관련 필터의 integration(비-race) 통과. tools 106.151초, server 3.320초. 첫 core 실행에서 기존 테스트 3개가 `/var`와 `/private/var` 문자열 차이로 실패했다. source/core 코드는 바꾸지 않고 `TMPDIR=/private/tmp/pixel-export-budget/mac-temp`로 실패 3개와 같은 core 필터를 재실행해 통과했다(최종 aseprite 0.983초). 최초 실패 로그도 보존했다.
- 지원 하한은 정렬 차이 발견으로 필요한 범위만 추가 실행했다. 별도 `pixel-export-budget-min` / Go 1.25.14 / Aseprite `1.3.17.2`에서 24개 equal-area frame의 수정 전 `spritesheet allocation plan mismatch`를 실제 재현했다. 수정 후 `-tags=integration ./pkg/tools -run 'TestExportBudget|TestExportOptionsBackground' -v`가 39.015초에 통과했다. 하한 전체 suite 반복은 아니다.
- 큰 실제 bitmap을 만들지 않은 계획 경계/overflow/65535 endpoint/padding/면적·계산량·cancel 검사와 occupancy-grid oracle의 결정적 500개 입력을 통과했다. 새 native oracle는 3모드×5layout×trim on/off, mixed/empty/different-size samples와 padding/extrude를 대조했다. Wide-canvas trim은 4형식×5layout을 검사했다. 구버전 packed는 크기를 보수 envelope 이하로, 최신 버전은 정확한 크기와 XY placement로 검증한다.
- 전용 config/temp와 timeout=30, GOMAXPROCS=2/-p 1을 사용했고 native workload는 순차 실행했다. 로그와 source archive/manifest는 `/tmp/pixel-export-budget/`에 있다(`red.log`, `native-budget2.log`, `repair1.log`, `min-red.log`, `min-green.log`, `linux-final.log`, `macos-final.log`, `macos-core-final.log`). 초기 oracle의 bound-context fixture 오류도 수정 후 재검증했다. 기본 사용자 설정·원본 develop·다른 워크트리·plugin을 수정하지 않았다.
- gofmt·diff 공백·변경 문서 상대 링크 검사, 실제 등록 이름 집합/64개 CAPABILITIES inventory 대조 통과. 최종 Linux/하한 로그에 FAIL/SKIP 없음; macOS는 위 경로 fixture 실패를 canonical temp 재검사로 해소했다.
- 로컬 전체 integration과 macOS 전체/race, 모든 format×blend×mode 조합 전수검사, 지원 하한 전체 suite는 실행하지 않았다. 변경 영향 범위와 하한 정렬 회귀에 집중했다. 원격 PR의 최종 HEAD CI 전체 suite 결과는 push 후 별도로 확인한다.

`convergent-code-review` backend lens, review-and-repair. 범위는 12개 파일(Go 7, 문서 5)의 base-to-candidate diff다.

| ID | 우선순위·최종 상태 | 원인·수정·검증 |
| --- | --- | --- |
| BE-BUDGET-001 | P2 · VERIFIED | 새 렌더 직전 guard가 Lua exception을 내면 공개 코드가 `file_changed` 대신 `lua_error`가 된다. 구조화된 `plan_changed` 결과를 Go에서 typed `file_changed`로 변환한다. 실제 source 수정 후 native 출력 파일 미생성 및 공개 분류를 검증했다(`repair1.log`, 최종 관련 검사). 성공 응답은 변경하지 않았다. |
| BE-BUDGET-002 | P1 · VERIFIED | 첫 fresh-discovery에서 하한의 `std::sort`와 최신 `std::stable_sort` 차이를 확인했다. 최신 placement를 하한에 강제하면 기존 정상 packed export가 실패한다. 24개 같은 면적·다른 모양 native red를 재현하고, 1.3.18.2 미만 packed의 trim/중복 기반 보수 envelope를 복원했다. 하한 green과 최신 Linux/macOS oracle를 대조했으며 하한의 잔여 과잉 거부를 명시했다. |

초기 discovery 1회, repair cycle 2회, repair-diff 2회, closure 2회, 전체 fresh-discovery 2회, 총 review pass 7회. 초기 P2 1건; 첫 repair/closure에서 신규 finding 0, 첫 fresh에서 P1 1건; 두 번째 repair/closure/fresh 신규 finding 0. Repair가 만든 finding·재개방 0, VERIFIED 2, OPEN/ACCEPTED/OUT_OF_SCOPE 0. 차단 P1은 initial/cycle 1의 0 → fresh 1의 1 → cycle 2/최종 closure/fresh의 0으로 수렴했다. 첫 후보 인증은 하한 finding으로 1회 무효화했다. 두 번째 cycle 뒤 실제 위험 감소와 미해결 차단 항목 없음을 확인하고 mutation을 종료했다. 전체 소요 시간은 별도 측정하지 않았다.

Reviewed: 계획·Lua 측정/재검사·native export·메타데이터 대조·회귀·현재 계약과 현황 문서. Inspected for context: Client/capabilities, file-protection/output-files/revision/history 및 기존 export/구조/서버 diagnostics/등록, 공식 native renderer/exporter/packing 구현. Changed during repair: plan Lua, allocator/runtime 분기, sheet 결과 decoder/metadata 검사, 회귀와 해당 계약. Excluded: plugin 변경, 공통 저장 core 재설계, crash 복구, indexed index 유지 옵션, GAP-04/GAP-06, 병합·태그·배포.

최종 후보(이 보고 추가 전) manifest SHA-256: `cf92dae671d2fc8ff2bd36d3d00a2ee6ddfac3c839918e6a831a5642c44ae7c4`. 마지막 관련 mutation 이후 closure/fresh와 최종 검증 동안 candidate 변경 없음. 이후 추가는 실행·리뷰 보고와 완료 상태 기록뿐이다.

Notes:

1. 1.3.18.2 미만 packed에는 보수 envelope와 일부 정상 요청의 거부가 남는다. 그 버전까지 exact packing을 완료한 것은 아니다.
2. 최신 packed도 native 알고리즘 변경 시 재검증이 필요하고, 복잡한 입력은 계획 계산량 제한으로 거부될 수 있다. Source decode를 포함한 전체 process RSS 상한을 제공하지 않는다.
3. 일반 오류 rollback과 crash 원자성의 차이, 비협력 writer 경쟁 및 위 전수검사 미실행 범위는 유지한다. Crash 복구·indexed/GAP 검토와 plugin 새 pin/번들 갱신은 후속이다.

로컬 최종 판정: **PASS_WITH_NOTES**. 최종 PR HEAD CI 확인과 develop 병합 승인은 별도 단계다.

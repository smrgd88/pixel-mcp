# 범위 export 및 출력 보호 (GAP-05)

작업: `[BE][FEATURE] 범위 export 및 출력 보호`, `feature/be-export-options`.
기준: `origin/develop` `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732` (PR #29/#30/#31 병합). 이 브랜치의 구현 후보이며 develop 병합·릴리스·plugin 배포와 구분한다. 도구 추가 없이 기존 **58개** 이름 집합을 유지한다. 다른 Top4 브랜치의 코드를 사용하지 않는다.

## 호환성과 옵션

기존 필수 입력, 성공 응답 필드, `frame_number=0`의 전체 export, PNG/JPG/BMP의 `stem_0001.ext` 번호 체계는 유지한다. 단일 출력의 `exported_path/file_size`, 시퀀스의 `files[{path,file_size,frame_number}]`, sheet의 `spritesheet_path/frame_count/metadata_path` 의미도 동일하다. 선택 sheet의 frame_count는 선택된 프레임 수다.

| 옵션 | export_sprite PNG/JPG/BMP | export_sprite GIF | export_spritesheet |
| --- | --- | --- | --- |
| tag, frame_start/end | 지원 | 거부; 기존 frame_number만 지원 | 지원 |
| layer_id, include_hidden | 지원 | 거부 | 지원 |
| expected_revision | 지원 | 지원 | 지원 |
| trim | 투명 여백을 프레임별로 자름 | true 거부 | native cel trim |
| extrude | 입력 없음 | 입력 없음 | bool, native 1px edge extrusion |
| border/shape/inner_padding | 입력 없음 | 입력 없음 | 각각 정수 0–100 |
| overwrite | 지원 | 지원 | texture+JSON 모두 적용 |

Sheet texture 확장자는 `.png/.jpg/.jpeg/.gif/.bmp`다. 확장자 없는 경로·native 파일·JSON 자체를 texture로 쓰는 요청은 거부한다. PNG는 lossless RGBA/indexed 검증 대상이고 JPEG는 lossy, GIF/BMP는 해당 native encoder의 색/alpha 제약을 유지한다. GIF animation 구조·재생 방향·반복 횟수 확장은 포함하지 않는다.

- `tag`: 저장된 native tag의 **정확한 이름**. 빈 이름·없는 이름·중복 이름은 거부한다. CLI wildcard/경로/예약 문자열을 해석하지 않는다. tag의 inclusive from/to를 오름차순으로 한 번씩 export하며 reverse/pingpong을 재생 순서로 확장하지 않는다. GAP-03 조회/수정 도구가 필요하지 않다.
- `frame_start`, `frame_end`: 각각 생략/null은 1/마지막 frame, 명시적 값은 1–65535의 정수. 0·음수·역순·문서 범위 초과는 오류. `tag`, frame range, 양수 `frame_number`는 상호 배타적이다. `frame_number=0`은 새 범위 선택과 함께 쓸 수 있다.
- `layer_id`: [get_sprite_structure](SPRITE_STRUCTURE.md)의 형제 index 경로와 `expected_revision`을 함께 전달한다. 이름 필터는 받지 않으므로 동일 이름·슬래시·따옴표·줄바꿈으로 다른 레이어를 선택하지 않는다. group은 하위 전체를 선택하며 부모 blend/opacity를 유지한다. 없는 ID는 오류다.
- 기본 visibility는 저장된 target/조상의 hidden 상태를 모두 반영한다. `include_hidden=true`는 layer_id가 필요하며 target·선택 하위·조상만 임시로 보이게 한다. 선택 밖 sibling은 포함하지 않는다. locked는 읽기 export를 막지 않는다. GUI 선택 상태는 사용하지 않는다.
- `expected_revision`: optional SHA-256 내용 token. layer_id에는 필수이고 다른 선택에도 사용할 수 있다. 구조나 파일 bytes가 바뀌면 `file_changed`로 거부하므로 새 구조 조회가 필요하다.
- `trim=false`, `extrude=false`가 기본. 기존 `padding` 값은 border/shape/inner 세 값의 기본값이다. 개별 padding의 생략/null은 fallback, **명시적 0은 0으로 덮어쓴다**. shape는 shape 사이 여백, border는 sheet 바깥 여백, inner는 frame 안 투명 inset이다. native frame bounds에는 inner가 포함되고 extrude rim은 포함되지 않는다. inner>0이면 바깥 edge가 투명하므로 extrusion도 투명할 수 있다.
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

Canvas/결과 한 변 32767 및 64 Mi pixels의 보수적 예산을 적용한다. trim 전 canvas와 padding/extrusion을 포함한 strip 크기를 사전 검사한다. rows/columns/packed는 가능한 가로·세로 strip extents의 둘레 사각형으로 예산을 검사하므로 실제 packing하면 들어갈 큰 요청도 거부할 수 있다. 더 작은 범위로 나눠 export한다. timeout은 30초 기본을 유지한다.

일반 오류 rollback은 **세트 전체의 crash 원자성**이 아니다. 협력하는 호출은 경로 잠금을 통해 완성된 세트를 관찰하지만 외부 reader는 교체 도중 일부 새 파일을 볼 수 있다. 비협력 writer의 ABA/최종 검사 이후 경쟁, 강제 종료·전원 장애 자동 복구 및 고아 staging 정리는 후속이다. Source history는 출력 세트 undo가 아니다. GAP-06 slice/pivot/nine-slice 및 편집 도구는 이번 범위 밖이며 기존 native slice metadata를 확장/재해석하지 않는다.

오류 코드는 [ERRORS](ERRORS.md)를 따른다. Go 입력/예산/overwrite 검사는 invalid_arguments, stale source는 file_changed, 기존 frame_number 범위 오류는 invalid_arguments를 유지하고, 새 native tag/layer/range/blank 검사는 lua_error, 파일·cancel·rollback은 기존 typed 분류다. 새 공개 오류 코드를 추가하지 않는다.

## 검증과 plugin 후속

테스트: `TestExportOptions*`는 실제 MCP/Aseprite 저장·재열기, RGB/grayscale/indexed mask=3와 opaque index 0, native linked/positioned cels, frame palette 변화, 그룹/숨김/중복 이름, tag/range/빈 frame, stale/invalid/aliases/overwrite/폴더/read-only/cancel/source 변화, 독립 PNG 전체 픽셀·JSON bounds/duration/trim/extrude 및 history/snapshot/undo를 검사한다. 순서/번호 및 JSON escape는 deterministic testing/quick 각 1000개 입력으로 검사한다. 기존 `TestExportSequence*`, `TestOutputFiles*`/`TestOutputRollback*`와 server diagnostics/등록 검사를 함께 수행한다. 실제 실행 결과는 아래 최종 기록으로 구분한다.

Plugin 저장소·MCP commit pin·번들 바이너리는 이 작업에서 수정하지 않는다. 이 PR 병합 후 별도 동기화에서:

- 기존 export_sprite/export_spritesheet의 optional 입력 schema와 새 서버 commit을 반영한다. 도구 이름/수 추가는 없다.
- exporter skill에 `get_sprite_structure → layer_id+revision`, tag/range 배타 규칙, hidden/blank 정책, padding override, overwrite=false, 범위의 원본 번호 규칙을 추가한다.
- sidecar source_frame_number와 출력 기준 frameTags, include_json=false의 legacy sidecar, texture+JSON rollback 및 recovery 순번을 소비자/스킬에 반영한다.
- crash 원자성이나 출력 history undo를 주장하지 않는다. GIF 새 선택은 지원하지 않는다.

동시 PR은 CAPABILITIES/NEXT_STEPS/ROADMAP/CHANGELOG 및 한·영 README가 충돌할 수 있다. 이 브랜치는 animation/server production 파일을 수정하지 않으며 미병합 Top1–3을 완료로 표시하지 않는다.

근거: [ExportSpriteSheet API](https://www.aseprite.org/api/command/ExportSpriteSheet/), [Image API](https://www.aseprite.org/api/image/), Aseprite 1.3.18.3 native `cmd_export_sprite_sheet.cpp`, `layer_frame_comboboxes.cpp`, `doc_exporter.cpp`. 공식 API의 존재와 이 후보의 실제 검증 범위를 구분한다.

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

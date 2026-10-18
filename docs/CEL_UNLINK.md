# Cel unlink (GAP-02)

작업: `[BE][FEATURE] cel unlink`, `feature/be-cel-unlink`.
기준: `origin/develop` `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732` (PR #29 nested drawing, #30 구조 조회, #31 cel 속성 병합 포함).
현재 후보는 Unreleased/미병합이다. 조회·위치/opacity는 develop 반영 완료이며, 이 문서는 추가 `unlink_cel`의 계약과 검증만 다룬다. 다른 Top4 기능·plugin·배포는 포함하지 않는다.

## 호출과 응답

먼저 `get_sprite_structure`로 `layer_id`, frame과 `revision`을 조회한다.

```json
{"sprite_path":"/absolute/sprite.aseprite","layer_id":"2/1","frame_number":1,"expected_revision":"<64자리 소문자 SHA-256>"}
```

네 필드 모두 필수다. `.ase`/`.aseprite`만 허용하며 대소문자 확장자는 허용한다. symlink 해석 후 **실제 저장 파일명**도 native 확장자여야 한다. `layer_id`는 양의 1-based 형제 index 경로이며 선행 0·이름 경로·Lua 문자열 주입을 거부한다. `frame_number`는 1…65535 정수이고 실제 frame/cel이 존재해야 한다. 0·생략·null은 기본 frame으로 보정하지 않는다.

성공 응답 예:

```json
{"success":true,"revision":"<저장된 파일의 새 SHA-256>","cel":{"layer_id":"2/1","frame_number":1},"remaining_linked_cels":[{"layer_id":"2/1","frame_number":2},{"layer_id":"2/1","frame_number":3}]}
```

`cel`은 이제 독립 이미지다. `remaining_linked_cels`는 원래 집합에서 대상만 뺀 전체 멤버로, 계층 depth-first/레이어 cel 순서다. 원래 두 멤버였다면 배열의 남은 하나도 **독립 이미지**다. 응답 명칭이 그 하나에 링크가 남았다는 뜻은 아니다. 새 revision으로 다시 조회해 다음 편집 대상을 결정한다. 기존 도구의 입력·성공 payload·warnings는 바꾸지 않는다. `set_cel_properties`는 계속 unlink하지 않는다.

## 대상과 불변식

- 일반 raster cel만 분리한다. root/nested, 중복 이름, 숨김 레이어/숨김 조상, 음수·off-canvas 위치를 지원한다. 이름 검색·GUI 선택 상태로 대상을 추측하지 않는다.
- 대상 또는 조상이 locked이면 거부한다. group/tilemap/reference/background 대상, 없는 ID/frame/cel을 거부한다. 비대상 locked/hidden 레이어를 해제하거나 이동하지 않는다.
- **이미 독립이면 오류로 거부**한다 (`lua_error`, 상세 진단: `Cel is already independent`). 원본 저장·history 생성은 없다. 같은 픽셀을 가진 다른 image를 링크로 판단하지 않는다. legacy linked header가 native decoder에서 copy로 읽히면 독립으로 취급한다.
- `Image.id`는 한 batch 안의 native 공유 판별에만 사용하며 응답에 노출하지 않는다. 색상 모드 변환·픽셀 재해석·cel 이동·다른 cel 복제/삭제 없이 선택 cel의 shared CelData/image를 공식 명령으로 복사한다.
- 픽셀 bytes/크기/모드, palette/transparent index, position/opacity/zIndex, cel data/color/properties, layer/sprite metadata, 계층·visibility/editability, tag와 frame duration을 보존한다. 저장 전후 모든 cel의 이미지 공유 partition·픽셀·기본 cel metadata를 runtime 검사한다. 확장 properties와 palette/tag 등은 실제 회귀의 보존 검사 및 native command/save 경로에 의존하며, 미지의 future chunk·모든 extension 또는 ACL/xattr byte 보존까지 보장하지 않는다.
- 스캔·검증은 조회 페이지 밖을 포함한 문서 전체다. 이미지 bytes는 native image마다 한 번 보관한다. 전체 문서 크기와 cel 수에 따른 메모리/재열기 비용이 있고 응답 멤버 수 별도 상한은 없다. 기본 timeout 30초를 유지한다.

## 공식 native 명령과 공유 범위

[공식 app.command 목록](https://www.aseprite.org/api/app_command/)의 `UnlinkCel`을 사용한다. 실제 지원 버전 [명령 소스](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/app/commands/cmd_unlink_cel.cpp)는 선택 cel의 잠금 상태를 확인하며, [복사 구현](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/app/cmd/unlink_cel.cpp)은 image와 CelData/user data를 복사한다. pixel setter나 `Sprite:newCel`로 대체하지 않는다. `app.transaction` 안에서 active layer/frame과 range를 **한 cel**로 설정하고 호출한다. 새 batch이므로 사용자 GUI의 timeline 범위를 재사용하지 않는다. transaction 내부와 save/reopen 이후 partition을 검사해 명령이 no-op이거나 다른 멤버를 분리하면 발행하지 않는다.

native `.aseprite`의 링크는 같은 레이어의 다른 frame을 가리킨다. [파일 규격](https://github.com/aseprite/aseprite/blob/v1.3.18.3/docs/ase-file-specs.md#cel-chunk-0x2005)에는 link frame만 있고, [decoder](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/dio/aseprite_decoder.cpp#L876)는 현재 layer의 cel을 찾는다. `Sprite:newCel`도 입력 image를 복사한다. 따라서 **서로 다른 그룹/레이어 사이의 단일 native 공유집합을 저장하는 기능을 주장하지 않는다**. 그룹 내부에서 frame을 넘는 링크와 여러 그룹 각각의 링크 집합을 동시에 만들고 비대상 집합 보존을 검사한다. 전체 문서 partition 검사는 뜻하지 않은 공유 합침/분리를 막는다.

## 오류·원본 보호·복구

기존 per-file/source/history 잠금 → `WithSpriteAccess` staging → 실제 bound bytes의 expected revision 검사 → Lua transaction/저장/재열기 → 원본 변경 감지/atomic publication을 재사용한다. 주소가 같은 다른 layer/frame으로 바뀐 경우에도 파일 revision이 다르면 `file_changed`로 거부한다. 자동 retry로 다른 대상을 수정하지 않는다. 내용 hash는 영구 UUID나 sequence가 아니며, byte-exact 복원은 같은 revision이다. 비협력 writer의 ABA 및 최종 검사/rename 경쟁 한계는 [CEL_PROPERTIES](CEL_PROPERTIES.md#오래된-구조동시-변경-방어)와 [FILE_PROTECTION](FILE_PROTECTION.md)을 따른다.

MCP schema 타입/필수 필드 오류와 Go `invalid_arguments`, 누락 파일 `not_found`, stale `file_changed`, 대상/독립/잠금/저장 결과 오류 `lua_error`는 기존 [ERRORS](ERRORS.md) 경로다. 새 공개 오류 코드를 추가하지 않는다. request ID/redaction/cancel/timeout은 공통 서버 계약을 따른다. 실패·취소 시 원본을 발행하지 않고 외부 writer의 변경을 되돌리지 않는다.

`enable_history=true`에서 성공한 unlink만 `unlink_cel` operation과 편집 전 snapshot을 기록한다. [HISTORY](HISTORY.md)의 expected operation ID undo 및 [SNAPSHOTS](SNAPSHOTS.md)의 수동 restore로 원래 bytes와 공유를 복원한다. 기본 비활성 설정에서는 자동 복원본을 만들지 않는다. process 간 native undo나 crash-atomic 파일 저장을 약속하지 않는다.

## Plugin 반영 및 통합

- additive 도구: `unlink_cel`; 필수 네 입력과 위 `success/revision/cel/remaining_linked_cels` 응답을 반영한다. 기존 도구 변경 없음.
- Top1~3 develop 병합 후 MCP commit pin·번들·도구 목록을 한 번에 동기화할 때 animation skill에 구조 조회 → revision guard unlink → 새 revision 조회 → 단일 cel 편집 흐름을 추가한다. 독립 cel 거부와 한 peer가 남으면 둘 다 독립임을 설명한다. Top4 export는 별도 plugin 동기화다.
- pixel-plugin 저장소·bundled binary는 이 작업에서 수정하지 않는다.
- 다른 Top4 PR과 README 한·영, CAPABILITIES/NEXT_STEPS/ROADMAP/CHANGELOG, animation 등록 및 server count tests 충돌 가능성이 있다. 병합 전 최신 develop의 실제 이름 집합으로 수를 다시 계산해야 한다. 현재 기준 58개 + 이 후보 1개 = 59개. 다른 미병합 브랜치를 합치지 않는다.

## 검증 기록

실행 결과 및 셀프리뷰는 최종 후보 검증 뒤 아래에 기록한다. 과거 환경 실패는 기능 실패와 구분한다.

### 2026-10-04 실행 결과

- Linux amd64: 전용 Docker `pixel-cel-unlink`, image `pixel-mcp-ci:latest` (`b0b99e1a1e34`), Go 1.25.14, 실제 Aseprite `1.3.18.3-dev` / API 41. container 내부 `/workspace`·`/tmp`, 별도 `/tmp/unlink-config.json`, timeout 30초. 호스트 부하를 확인하고 `--cpus=2`, `GOMAXPROCS=2`, `-p 1`, `-parallel 1`로 각 suite를 순차 실행했다.
- `go build -p 1 -o /tmp/pixel-mcp ./cmd/pixel-mcp`, `go vet -p 1 ./...`, CLI `--health`: 통과.
- `go test -p 1 -parallel 1 -count=1 -race -cover ./...`: 통과. aseprite 82.7%, tools 62.6%, server 90.7%. 기본 suite의 statement coverage이며 조합 완성률이 아니다.
- `go test -p 1 -parallel 1 -count=1 -tags=integration ./...`: 전체 통과. tools 515.163초. 새 unlink와 기존 cel 속성·구조·drawing·export·복구 회귀 포함. 새 도구만의 초기 Linux 검증도 79.556초 통과했다.
- 최종 영향 범위: `go test -p 1 -parallel 1 -count=1 -race -tags=integration ./pkg/aseprite ./pkg/tools ./pkg/server -run "TestUnlinkCel|TestSpriteStructure|TestStructureInput|TestSnapshotToolsRegistered|TestToolDiagnostics|TestSpriteRevision" -v`: 통과. 새로운 동시 요청/분리/복구와 구조 조회·서버 경계를 race 옵션으로 재확인했다.
- macOS arm64: 캐시된 실제 Go 1.25.0과 Aseprite `1.3.18.2-arm64` / API 41. 전용 config/temp 및 canonical `TMPDIR=/private/tmp/pixel-unlink-evidence/mac-temp` 사용. build/vet/health 통과. 신규 `TestUnlinkCel` 초기 검사 48.137초, 추가 special/subsequent-edit·server 오류/등록 검사 통과. 확장 검사에서 aseprite의 revision/file protection/history/snapshot, tools의 기존 cel 속성/drawing/matrix/복구, server의 성공/오류/request ID/redaction/schema/등록 검사가 통과했다. 아래 두 조회 실패와 재확인 결과를 별도로 보존한다.
- 확장 macOS 검사 중 `TestUnlinkCelSavedModesAndRecovery/rgb/2/frame1`의 undo 후 구조 재조회(33.75초 case) 및 기존 `TestSpriteStructureSavedModes/rgb` 조회(30.90초 case)가 실패했다. 당시 공용 테스트 helper는 오류 본문 대신 pointer 목록만 출력해 정확한 오류 코드를 복원할 수 없다. timeout 의심 관찰이며 원인을 코드나 호스트 부하로 단정하지 않는다. 사용자 요청으로 helper가 실제 오류 응답 JSON을 남기도록 보강한 뒤 **같은 timeout 30초**로 두 case를 재실행해 각각 5.98초/4.98초에 통과했다(패키지 11.496초). 실패를 skip하거나 timeout을 늘리지 않았다. 새 helper의 최종 Linux 검증도 위 suite에 포함된다.
- 재개 전 sandbox에서는 Docker 소켓·GitHub DNS 접근이 실패했고 macOS Aseprite는 편집 코드 진입 전 `_RegisterApplication/NSApplication sharedApplication`에서 종료했다. 사용자가 확인한 시스템 로그의 WindowServer/LaunchServices mach-lookup deny와 별개인 기능 결함으로 분류하지 않는다. 사용자 승인으로 실행 환경을 복구한 후 version/health/native/Docker 검사를 다시 수행한 결과가 위 기록이다. 글로벌 설정·다른 작업의 권한은 수정하지 않았다.
- `gofmt -l`, `git diff --check`, 변경 Markdown의 로컬 링크 검사, 실제 등록 이름 집합과 README 한·영/CAPABILITIES의 59개 inventory 대조 통과. 변경 Go 8개 파일의 host/container SHA-256 일치 확인.
- 별도 PBT/fuzz framework/suite는 저장소에서 찾지 못했다. 세 모드·멤버 수·anchor 순서의 결정적 fixture에서 native identity, 저장된 pixels/metadata, 전체 렌더 bytes 및 원본 파일 불변식을 검사했다. 새 명시적 검사는 skip 없이 실행했다. 기존 full suite의 조건부 skip은 전수 감사하지 않았다. macOS 전체/race 및 모든 metadata·특수 레이어 혼합 조합 전수 검사는 미실행이며, 하한 전체 suite는 릴리스 전/명시 요청 정책에 따라 반복하지 않았다.
- 임시 증거: `/private/tmp/pixel-unlink-evidence/`의 `linux-full.log`, `linux-final.log`, `linux-unlink.log`, `mac-unlink.log`, `mac-additional.log`, `mac-regression.log`(두 최초 실패 포함), `mac-timeout-recheck.log`, `final-candidate.json`. 영구 배포 artifact는 아니다.

### 셀프리뷰

`convergent-code-review` backend lens, review-and-repair 범위: base/initial HEAD/merge-base `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732`부터 최종 미커밋 후보의 21개 파일(Go 8, 문서 13). 초안 candidate 이후 사용자 요청으로 테스트 실패 진단 출력을 보강했고, 그 변경을 포함해 최종 후보를 다시 고정했다. 최종 후보 manifest SHA-256: `d5ebe07a9961afd3c21a66d03b0819be21537873fc5dece8e4ecfcbb0011725e`. 이후 변경은 이 실행·리뷰 보고뿐이다.

입력/schema → 정확한 구조 주소와 staged revision → 한 cel의 native command 선택 → 전체 공유 partition/픽셀/metadata 검증 → save/reopen → history/atomic publish → 서버 오류·성공 계약을 재검토했다. 기존 지적 목록이나 테스트 통과를 traversal 목록으로 삼지 않고 최종 전체 diff에서 새로운 실패 경로도 검토했다.

- 활동: 초기 discovery 1회, 사용자 요청에 따른 test-only 진단 mutation 1회, 해당 diff review 1회, closure 1회, fresh-discovery 전체 범위 1회. 총 review pass 4회. production finding repair는 0회. 최종 후보 선택 후 관련 변경/인증 무효화 0회. 총 소요 시간은 별도 측정하지 않았다.
- finding ledger: 초기 P0/P1/P2/P3 0건. mutation 이후·closure·fresh 신규 지적, repair 유발 결함, 재개방, accepted/out-of-scope/미해결 finding 모두 0건. 진단 출력 보강은 사용자 요청에 따른 검증 지원이며 새 제품 결함의 수정으로 집계하지 않는다. P0/P1은 전 단계 0건이다.
- Reviewed: 새 도구/generator·animation 등록·server schema/diagnostics/count·unlink 회귀·structure 테스트 helper·관련 문서.
- Inspected for context: 기존 cel 속성/structure 입력과 fixture, 공통 wrapper/Client, revision/lock/staging/atomic replace, history/snapshot, 공식 native command/decoder. 이들 모듈 전체 audit를 주장하지 않는다.
- Changed during review: `pkg/tools/structure_integration_test.go`의 실패 진단 출력만 보강. production 변경 없음.
- Excluded: 다른 Top4 구현, plugin 저장소·번들, GUI 지속 세션, branch 병합/main/태그/배포, 관련 없는 리팩터링.

Notes:

1. macOS의 두 일시적 조회 실패는 재실행 통과했으나 최초 원인은 미확정이다. 재발 시 이제 오류 본문을 확인할 수 있다. 위 미실행 범위와 구분한다.
2. native의 같은 layer/frame 간 링크 범위, 전체 문서 순회/이미지 bytes/재열기 비용 및 비협력 writer의 ABA·최종 검사 경쟁은 위 계약을 따른다.
3. 다른 Top4 PR의 공통 등록/count/문서 충돌은 통합 시 조정해야 한다. plugin pin/번들/animation skill 반영은 후속이며, 미병합 브랜치·워크트리는 정리 대상이 아니다.

판정: **PASS_WITH_NOTES**. 최종 후보 검증·closure·fresh review가 끝났고 알려진 범위 내 차단 결함이 없어 추가 mutation을 하지 않는다. 외부 독립 리뷰·develop/main 병합·태그·배포 완료를 뜻하지 않는다.

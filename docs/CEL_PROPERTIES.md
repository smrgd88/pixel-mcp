# Cel 위치·불투명도 편집 (GAP-02 후속)

작업: `[BE][FEATURE] cel 위치 및 불투명도 편집 + 관련 문서 최신화`.
기준: `origin/develop` `eca9e836c44a5dd0563372d3dd61cb7801cb2a8f` (PR #29 RM-FIX-01 및 PR #30 상세 조회 병합 포함).
현재 상태: PR #31로 develop `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732`에 병합 완료 / Unreleased. 아래 검증·리뷰 기록은 당시 이력이며 태그·배포 완료와 구분한다. 플러그인 저장소와 bundled binary는 이번 범위 밖이다.

## 호출 계약

먼저 `get_sprite_structure`로 구조와 **revision**을 조회하고, 다음 요청에 그대로 전달한다.

```json
{
  "sprite_path": "/absolute/sprite.aseprite",
  "layer_id": "2/1",
  "frame_number": 1,
  "expected_revision": "<get_sprite_structure가 반환한 64자리 소문자 SHA-256>",
  "x": 0,
  "y": -3,
  "opacity": 128,
  "allow_linked": true
}
```

| 입력 | 계약 |
| --- | --- |
| `sprite_path` | 필수. `.ase`/`.aseprite` 저장 파일. 요청 경로와 symlink 해석 후 실제 저장 경로 모두 native 확장자여야 함. 대소문자 확장자 허용 |
| `layer_id` | 필수. 조회의 1-based 형제 index 경로. 중복 이름·그룹 내부 대상을 이름 검색 없이 선택. 선행 0·이름 경로 거부 |
| `frame_number` | 필수. 1…65535 범위 정수이며 실제 문서 frame 안에 기존 cel이 있어야 함 |
| `expected_revision` | 필수. 조회 시 파일 전체 bytes의 SHA-256. 잘못된 형식 거부, 불일치는 `file_changed` |
| `x`, `y` | optional integer/null. 생략/null은 유지, 명시적 0은 0. sprite-absolute 원점, −32768…32767. 음수·화면 밖 허용. native signed SHORT 저장 범위이며 clip/resize 없음 |
| `opacity` | optional integer/null. 생략/null은 유지, 명시적 0은 완전 투명. 0…255만 허용 |
| `allow_linked` | 기본 false. true일 때만 native image 공유 집합 전체 속성 변경 허용 |

최소 한 속성을 지정해야 한다. 모든 지정값이 현재값과 같은 요청은 오류로 거부하고 save/history를 수행하지 않는다. 동일 요청 재전송은 기존 revision이면 `file_changed`, 새 revision이어도 변경이 없으면 `lua_error`다. 자동 retry로 다음 대상을 수정하지 않는다.

기존 `get_sprite_info` 및 다른 편집 도구의 필수 입력·성공 필드는 바꾸지 않았다. `get_sprite_structure`에는 `revision` 출력 하나만 추가했다. 기존 mutation 도구가 구조 ID를 받는다는 뜻은 아니다.

## 대상 및 공유 범위

- 일반 raster cel만 편집한다. root/nested·중복 이름 지원. 없는 ID/frame/cel은 오류이며 cel/레이어/frame을 생성하지 않는다.
- group, tilemap, reference, background 레이어를 거부한다. 대상 레이어 또는 조상 중 `isEditable=false`가 있으면 거부한다. 공유 집합의 다른 멤버에도 같은 정책을 적용한다.
- 숨김 레이어와 숨김 조상의 cel은 명시 대상으로 편집할 수 있다. visibility/editability 및 계층·순서를 바꾸지 않는다. GUI 선택·timeline 상태와 무관하다.
- `Image.id`는 한 batch 안에서 공유 집합 판별에만 쓴다. 픽셀이 같아도 별도 native image이면 비대상이다. native decoder가 이미 copy로 읽은 legacy linked header도 독립 이미지로 취급한다.
- linked cel 위치·opacity setter는 공유 CelData에 영향을 준다. 기본 거부 후 `allow_linked=true`가 있으면 **문서 전체 공유 집합**을 처리하며, 페이지/조회 frame 범위 밖 멤버도 포함한다. 단일 cel 편집을 위해 unlink하지 않는다.
- `app.transaction` 안에서 position/opacity만 지정한다. 저장 후 staging 파일을 재열어 전체 영향 집합의 위치·opacity·z-index, 이미지 bytes/크기와 native sharing을 검사한다. 검증 실패 시 발행하지 않는다.

응답은 `success`, 저장된 bytes의 새 `revision`, 전체 `affected_cels` (`layer_id`, `frame_number`, `x`, `y`, `opacity`)다. 배열은 계층 depth-first/각 레이어 cel 순서다. 공유 멤버가 2개 이상이면 기존 `{code,message}` 형식의 `linked_cel_properties` warning도 반환한다. 공유 집합 목록은 페이지로 나누지 않으므로 큰 애니메이션에서는 응답 크기와 전체 cel 순회·재열기 비용이 증가한다. 기본 timeout 30초를 유지한다. 성공 응답은 공통 atomic publish가 완료된 뒤 전달한다. warning은 동의 요청이나 undo 기능이 아니다.

## 오래된 구조·동시 변경 방어

구조 ID/frame 번호는 영구 identity가 아니다. 삽입·삭제·이동·재정렬뿐 아니라 파일의 어떤 byte가 달라도 revision이 달라진다. 이름과 위치가 같은 다른 대상이 생겨도 stale 요청을 거부한다. revision은 내용 token이며 인증·영구 UUID·단조 증가 sequence가 아니다. byte가 완전히 같은 파일 복사나 byte 단위 복원은 같은 revision이다.

조회는 기존 per-file lock 아래 원본을 읽고 Lua 전후 해시를 비교한다. 서로 다른 페이지의 revision을 비교해 변경을 발견하면 처음부터 재조회한다. 페이지 snapshot을 서버에 보관하지 않으며, 조회가 원본을 저장하거나 history를 생성하지 않는다. 해시를 위한 파일 전체 읽기 비용이 추가된다.

편집은 source/history 잠금 → 기존 `WithSpriteAccess` staging → **실제 Lua가 열 staged bytes의 revision 검사** → 실행/재열기 검증 → 기존 원본 변경 감지/atomic replace 순서다. precondition 검사 후 새 원본을 복사하는 틈을 만들지 않는다. 협력하는 같은 사용자/호스트 서버의 동시 요청은 직렬화되며, 같은 revision으로 경쟁한 요청은 최대 하나만 변경할 수 있다.

GUI 등 비협력 writer를 파일시스템 CAS처럼 배제하지는 않는다. 조회 전후 해시 사이의 일시적 변경 후 원복(ABA), 최종 원본 검사와 rename 사이 경쟁은 일반 잠금만으로 배제하지 못한다. 외부 편집과 동시 사용하지 말고 저장 완료 후 다시 조회한다. 같은 bytes로의 복원은 과거 조회와 같은 의미 상태로 인정한다. [파일 보호의 보장 경계](FILE_PROTECTION.md)를 유지한다.

## 오류·복구

입력 형식은 MCP schema/RPC, Go 범위 검사는 `invalid_arguments`, stale revision은 `file_changed`, 실제 저장 경로의 비native 확장자/대상/잠금/공유 동의/변경 없음/저장·재열기 검증 실패는 기존 `lua_error` 경로다. 누락 파일은 `not_found`. 기존 request ID, 정적 공개 오류 메시지, redaction, capability 검사 및 timeout/cancel을 사용한다. Lua 상세 이유는 debug 진단이고 공개 오류에 경로·사용자 metadata를 노출하지 않는다.

오류·취소·precondition 불일치 시 이번 변경을 원본에 발행하지 않는다. 외부 writer가 만든 bytes는 되돌리지 않는다. `enable_history=true`이면 성공한 실제 변경만 `set_cel_properties` 이력으로 기록한다. 편집 전 snapshot, 용량/TTL, undo 전 백업과 expected operation ID 계약은 [HISTORY](HISTORY.md) 그대로다. 비활성 기본값에서는 자동 undo 사본을 만들지 않는다. Lua transaction은 process 간 native undo나 전원 장애 복구가 아니다.

## 근거·제외 범위

[Cel API](https://www.aseprite.org/api/cel/)의 position/opacity, [Layer API](https://www.aseprite.org/api/layer/)의 타입/editability/parent, [Sprite API](https://www.aseprite.org/api/sprite/)의 saveAs 및 [native 파일 규격](https://github.com/aseprite/aseprite/blob/v1.3.18.3/docs/ase-file-specs.md#cel-chunk-0x2005)을 확인했다. 실제 지원 버전 소스의 Cel setter는 SetCelPosition/SetCelOpacity command를 사용한다. 공유 관찰·decoder copy 제한은 [SPRITE_STRUCTURE](SPRITE_STRUCTURE.md)를 따른다.

픽셀 편집·image resize·layer 속성·cel unlink·tag 편집·색상 모드 변환·export 추가는 제외한다. Aseprite가 지원하는 저장 metadata의 보존을 검사하지만 미지의 future native chunk나 모든 extension metadata, ACL/xattr 전체의 byte 보존을 약속하지 않는다. GAP-02는 부분 완료이며 unlink와 GAP-01은 후속이다.

## 검증

실행 결과와 셀프리뷰는 아래 최종 기록을 따른다. 새 회귀는 세 모드의 저장·재열기, root/nested/중복 이름, boundary/zero/null/invalid 입력, native 링크와 독립 동일픽셀 이미지, stale·중복·동시 요청, 잠금/숨김/특수 레이어 거부, metadata/order/palette 보존, render 및 byte-exact undo를 검사한다. 실패 주입은 같은 공통 wrapper에서 실제 Lua 저장 뒤 오류·취소·외부 변경·snapshot 용량 부족을 검사한다. fixture 생성에 사용한 NewLayer reference는 바닥에 삽입되므로 저장된 구조를 기준으로 검사한다.

### 2026-10-04 실행 결과

- Linux amd64 Docker: 전용 `pixel-cel-properties`, 기존 `pixel-mcp-ci:latest` image `b0b99e1a1e34`, Go 1.25.14. 실제 CLI/health는 Aseprite `1.3.18.3-dev`, API 41. `/tmp/cel-config.json`, 전용 temp, timeout 30초. `GOMAXPROCS=2`, `-p 1`로 순차 실행했다.
- `go build -p 1 -o /tmp/pixel-mcp ./cmd/pixel-mcp`, `go vet -p 1 ./...`, CLI `--health`: 통과.
- `go test -p 1 -count=1 -race -cover ./...`: 전체 통과. pkg/aseprite 82.8%, pkg/tools 63.2%, pkg/server 90.7%. statement coverage이며 기능 조합 완성률이 아니다.
- `go test -p 1 -count=1 -tags=integration ./...`: 전체 통과, pkg/tools 334.753초. 기존 drawing·구조 조회·history·snapshot·export 회귀 포함. 전체 실행 도중 아래 test-only 리뷰 보강이 있었으므로 최종 후보의 관련 검사를 별도로 다시 실행했다. production 코드 변경은 없었다.
- 최종 후보: `go test -p 1 -count=1 -race -tags=integration ./pkg/aseprite ./pkg/server ./pkg/tools -run 'TestCelProperties|TestSpriteRevision|TestSpriteStructure|TestStructureInput|TestRegressionMatrix|TestDrawPixelsLayer|TestSnapshotToolsRegistered|TestToolDiagnostics' -v`: 통과. pkg/tools 46.201초. stale 거부 사유·동시 요청·실패 주입·native 저장 결과를 포함한다.
- macOS arm64: 최종 소스를 Go 1.25.14에서 darwin/arm64로 cross-compile한 바이너리를 호스트에서 실행했다. 실제 Aseprite `1.3.18.2-arm64` / API 41 health 통과. 격리 config/temp 사용. tools의 `TestCelProperties|TestSpriteStructure|TestStructureInput|TestRegressionMatrix|TestDrawPixelsLayer`, server의 cel 성공/오류·request ID·redaction·등록 수, aseprite의 `TestSpriteRevision|TestFileProtection|TestHistory|TestSnapshot` 통과.
- macOS core 첫 실행은 기존 세 테스트의 기대 경로 `/var/...`와 canonical `/private/var/...` 문자열 비교로 실패했다. `TMPDIR=/private/tmp/pixel-cel-macos-canonical`로 같은 core 선택 전체를 재실행해 통과했다. 테스트를 제외하거나 production 경로 처리를 바꾸지 않았다. 최초 Linux 기본 검사도 config 환경변수 누락으로 실패했으며, 위 전체/최종 검사는 명시적 격리 설정을 사용했다.
- `gofmt -l`, `git diff --check`, 변경 문서의 로컬 상대 링크 검사 통과. README 한·영과 CAPABILITIES 모두 도구 58개 일치. 최종 Go 11개 파일의 host/container SHA-256 일치를 확인했다.
- 별도 PBT/fuzz suite는 저장소에서 발견하지 못했다. 고정 실제 native fixture와 경계값/관계/byte 불변식으로 검사했다. macOS 전체/race·앱 UI·특수 레이어의 편집/혼합 조합 전수 검사는 미실행이며, 지원 하한 전체 검사는 NEXT_STEPS 정책에 따라 반복하지 않았다. tilemap/reference/background **거부** 검증을 해당 기능 편집 지원으로 해석하지 않는다. 기본 suite의 모든 조건부 skip을 별도로 전수 감사하지는 않았다.
- 로그와 바이너리: 로컬 `/tmp/pixel-mcp-cel-evidence/`의 `linux-full.log`, `linux-focused-and-cross.log`, `macos-tools.log`, `macos-server.log`, `macos-aseprite.log`(최초 실패), `macos-aseprite-canonical.log`(재검증). 임시 로컬 증거이며 배포 artifact는 아니다.

### 셀프리뷰 (초기 후보 b487647의 이력)

`convergent-code-review` backend lens의 review-and-repair 모드로 base/initial HEAD/merge-base `eca9e836c44a5dd0563372d3dd61cb7801cb2a8f` 이후 변경 24개 파일(Go 11, 문서 13)을 검토했다. schema/정수 직렬화 → 구조 ID와 revision → native 공유 집합 → transaction/재열기 → snapshot/atomic publish → diagnostics/기존 소비자 계약을 대조했다.

| ID | 우선순위·최종 상태 | 근거·수정·검증 |
| --- | --- | --- |
| BE-P2-001 | P2 · VERIFIED | stale 구조 및 중복요청 테스트가 generic 오류만 확인해 locked/absent 오류로도 통과할 수 있었다. `cel_properties_integration_test.go`의 `rejectCel`에 예상 사유 검사를 추가하고 해당 호출이 `stale sprite revision`으로 거부됨을 명시했다. 최종 Linux race 및 macOS 실제 MCP 검사 통과. production 변경 없음 |

초기 discovery 1회, mutation cycle 1회(test-only), repair-diff review 1회, closure review 1회, fresh-discovery 전체 범위 review 1회, 총 review pass 4회. 초기 P2 1건 이외 P0/P1/P3는 0건. repair 이후·closure·fresh 신규 지적, repair 유발 결함, 재개방, accepted finding, 미해결 finding은 모두 0건이다. 차단 P0/P1은 전 단계 0건이며 P2는 1→0으로 해소했다. 필수 검사 및 최종 후보 전체 재검토가 끝나 추가 mutation을 하지 않는다. 소요 시간은 별도 측정하지 않았다.

- Reviewed: cel 도구/generator, bound revision helper, 조회 additive 출력, animation 등록과 server count/diagnostics, 회귀, 관련 문서.
- Inspected for context: 공통 wrapper/Client, history/snapshot/file staging, 기존 structure/drawing fixture와 native API·encoder/decoder. 이 문맥 코드의 전면 리뷰를 주장하지 않는다.
- Changed during repair: `pkg/tools/cel_properties_integration_test.go` 하나.
- Excluded: plugin/bundled binary, 릴리스, layer/unlink/tag/mode/export 추가, GUI, 무관한 리팩터링.

최종 후보 manifest(24 files)의 SHA-256은 `8952de53e9f82eacbd56826b58d839a8f2b328257a8b5263e5865cf8c4a46589`, Go 파일 manifest는 `7976f8d23abe7b96e7d9dfe03e35d85a4b660dad5b0e1cab1b6f0fe58107de88`이다. manifest는 로컬 evidence에 보존했다. 최종 후보 선택 후 관련 runtime/contract/test 변경 및 certification 무효화 0회이며 이후 추가는 이 실행·리뷰 보고뿐이다.

Notes:

1. revision은 byte 내용 token이다. 비협력 writer의 ABA/최종 검사 경쟁을 배제하지 않으며, GUI 저장과 동시 편집은 피하고 다시 조회해야 한다.
2. native load/hash/전체 공유 순회·재열기 비용과 공유 멤버 응답 크기 상한은 별도 보장하지 않는다. 대형 파일은 기본 timeout 내에서 처리돼야 한다.
3. 위 미실행 범위와 macOS 기존 테스트의 canonical temp 전제를 유지한다. 독립 외부 리뷰·원격 PR CI는 로컬 검증과 별개이며 develop/main 병합·태그·배포는 이 작업에서 수행하지 않는다.

판정: **PASS_WITH_NOTES**. GAP-02 전체 완료가 아니며, feature 브랜치와 워크트리는 미병합 상태이므로 정리하지 않는다.

### 추가 셀프리뷰 및 canonical 저장 형식 수정 (2026-10-04)

작업: `[BE][REVIEW] cel 속성 편집 재검토`. 기준/merge-base `eca9e83`, 시작 HEAD `b487647`. PR #31의 최초 HEAD에 대한 GitHub CI `test`가 통과한 것을 확인한 뒤, 이전 지적 목록과 별개로 PR 전체 24개 파일과 저장·식별·복구 경계를 다시 검토했다. 위 초기 리뷰에서 발견하지 못했던 다음 데이터 손실 경로를 실제 Aseprite로 재현했다.

| ID | 우선순위·최종 상태 | 재현·원인·수정·검증 |
| --- | --- | --- |
| BE-P1-001 | P1 · VERIFIED | `alias.aseprite` symlink가 `.png` 이름의 native 문서를 가리킬 때 요청 경로만 검사해 통과했다. 1-frame/2-layer 문서의 cel opacity를 128→255로 바꾸면 `saveAs`가 canonical staging 이름으로 PNG를 선택했고, 대상 cel만 검사하는 재열기 검증도 통과했다. MCP는 success=true였지만 layer_count는 2→1, 파일은 native→PNG로 바뀌고 숨김 레이어·문서 metadata가 소실됐다. `lua_cel_properties.go`에서 실제 `spr.filename`의 native 확장자를 mutation 전에 확인하도록 수정했다. `TestCelPropertiesCanonicalSaveFormat`으로 `.png/.gif/.bmp` target 거부 시 bytes/inode/mode/mtime/history 및 metadata 보존, `.ase/.aseprite/.ASE` target의 정상 저장·공유 alias 보존을 검사했다 |

- macOS 실제 Aseprite `1.3.18.2-arm64`에서 수정 전 MCP 재현을 수행했다. 임시 fixture만 사용했고 기존 사용자 문서는 변경하지 않았다. 자동 history가 꺼진 기본 설정에서도 손실되는 경로였다. 원인은 revision 일치나 atomic publication의 실패가 아니라, 이미 잘못된 형식으로 생성된 staging을 성공으로 간주한 것이었다.
- Linux Docker Go 1.25.14/Aseprite `1.3.18.3-dev`: 새 6-case 실제 회귀 통과(4.268초). 이어 `go build -p 1 ./...`, `go vet -p 1 ./...`, 초기 검증과 같은 관련 aseprite/server/tools 필터의 `go test -p 1 -count=1 -race -tags=integration ... -v` 통과(tools 50.264초). `GOMAXPROCS=2`, 격리 config, timeout 30초를 유지했다.
- 수정 소스로 macOS arm64 바이너리를 다시 빌드하고 `TestCelProperties|TestSpriteStructure|TestStructureInput`을 실행해 모두 통과했다. 새 canonical 파일명 검증과 기존 linked/undo/stale/오류·취소·history 회귀를 포함한다.
- 전체 suite/전체 coverage는 이번 좁은 수정에서 로컬 재실행하지 않았다. 앞선 전체 통과 및 최초 PR CI와 위 수정 후 관련 검증을 구분한다. 수정 커밋의 GitHub CI는 push 후 별도로 실행된다. macOS 전체/race·앱 UI·하한 전체 suite 미실행 정책은 유지한다.
- 초기 discovery 1회 → mutation cycle 1회 → repair-diff 1회 → closure 1회 → fresh-discovery 전체 1회, 총 review pass 4회. P1 1→0, P0/P2/P3 신규 0건, repair 이후·closure/fresh 신규 0건, repair 유발 결함·재개방·accepted/out-of-scope finding·미해결 0건. 기존 BE-P2-001은 재개방되지 않았다. 최종 후보 선택 이후 관련 변경/인증 무효화는 0회이며 이 결과 기록만 추가했다. 소요 시간은 별도 측정하지 않았다.
- Reviewed: 기준부터 cel 기능·조회 revision·등록/diagnostics·회귀·문서 전체(24개 파일). Context: 기존 canonical path와 staging filename binding, Client, snapshot/history/atomic replace. Changed during repair: generator, cel integration test, 본 계약 문서 3개. Excluded: 다른 편집 도구의 전면 audit, plugin/binary·릴리스, layer/unlink/tag/mode/export 확장. 알려진 같은 결함을 다른 도구에도 확인했다는 뜻은 아니다.
- 수정 후 최종 후보 manifest SHA-256은 `e1bfe7025209f7d87894f51151fe6970aa15c19ed885650890ba5ccbb57c2096`이다. 로그/manifest 및 수정 전 재현은 `/tmp/pixel-cel-rereview/`에 있다. `git diff --check`와 본 문서 상대 링크 검사 통과.

Notes: 비협력 writer의 ABA/최종 검사 경쟁, 전체 공유 집합 비용·응답 크기, 위 미실행 범위는 그대로다. 실제 저장 target의 확장자가 `.ase`/`.aseprite`가 아닌 alias는 이제 명시적으로 거부하며, native 이름으로 저장한 후 조회해야 한다. feature 브랜치/워크트리는 유지하고 develop/main 병합은 수행하지 않는다.

추가 리뷰 판정: **PASS_WITH_NOTES**. 재현된 P1을 수정·검증했으며 수정 diff와 전체 범위를 새로 검토한 결과 추가 차단 결함이 없어 종료한다.

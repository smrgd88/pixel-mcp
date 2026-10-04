# 태그 상세 조회·수정 (GAP-03)

작업: `[BE][FEATURE] 태그 상세 조회 및 수정`, `feature/be-tag-editing`.
기준: `origin/develop` `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732` (PR #29/#30/#31 병합). 이번 브랜치의 Unreleased 구현이며 develop 병합·태그·배포 완료가 아니다. 기존 도구 58개에 2개를 추가해 60개를 등록한다. cel unlink·layer/group·범위 export는 다른 세션의 독립 작업이며 여기서 완료로 표시하지 않는다.

## 호출 계약

`get_sprite_tags`에는 `sprite_path`만 필수다. 저장된 문서의 `revision` (전체 bytes SHA-256), `frame_count`, `tags` 배열을 반환한다. 태그가 없으면 `[]`이며 태그의 빈 이름·중복 이름·겹치는 범위를 그대로 반환한다. 각 항목은 `tag_id`, `name`, `from_frame`, `to_frame`, `direction`, `repeats`, `color`다. color는 소문자 `#rrggbbaa`다. 사용자 data/properties는 응답에 포함하지 않는다.

`tag_id`는 **현재 Aseprite tags 배열의 1-based 순번**이며 영구 ID가 아니다. 공식 구현은 시작 frame 오름차순, 같은 시작이면 끝 frame 내림차순으로 정렬한다. 동일 범위의 상대 순서는 native 동작을 따른다. 이름 순 정렬이나 생성 순서로 해석하지 않는다. 범위 변경만으로도 순번이 바뀐다. 이름은 ID가 아니며 중복·빈 이름도 순번으로 식별한다.

```json
{"sprite_path":"/absolute/sprite.aseprite"}
```

조회 직후 반환한 revision과 tag_id로 `set_tag_properties`를 호출한다.

```json
{
  "sprite_path":"/absolute/sprite.aseprite",
  "tag_id":2,
  "expected_revision":"<get_sprite_tags의 64자리 소문자 SHA-256>",
  "name":"walk",
  "from_frame":1,
  "to_frame":4,
  "direction":"pingpong_reverse",
  "repeats":0
}
```

| 입력 | 계약 |
| --- | --- |
| `sprite_path` | 필수. 요청 경로와 symlink 해석 후 실제 저장 파일명 모두 `.ase`/`.aseprite` (대소문자 무관) |
| `tag_id` | 필수 정수 1…65535. 현재 조회 결과의 순번. 없는 대상 거부 |
| `expected_revision` | 필수. 조회 revision과 실제 bound 작업 복사본 해시가 다르면 `file_changed` |
| `name` | 선택 문자열. 새 이름은 공백만/빈 문자열, NUL, 잘못된 UTF-8, 65535 bytes 초과 거부. 다른 태그와 같은 이름으로 **변경**하는 요청 거부. 정확한 문자열 비교이며 대소문자/Unicode 정규화나 trim을 적용하지 않음 |
| `from_frame`, `to_frame` | 선택 정수, 1-based inclusive. 1…65535 및 실제 frame 수 안. 생략된 끝점은 현재 값으로 평가하고 최종 역순 범위 거부. frame 0은 기본값이 아님 |
| `direction` | 선택 문자열. `forward`, `reverse`, `pingpong`, `pingpong_reverse`만 허용. 빈 문자열·대소문자 변형 거부 |
| `repeats` | 선택 정수 0…65535. native unspecified 값인 0은 UI 무한 반복/export 한 번(핑퐁은 각 방향 한 번). 생략과 0을 구분. 1 이상은 native 반복 횟수 의미 |

선택 속성의 생략/null은 유지한다. 최소 한 속성이 있어야 하며 실제 변화가 없는 요청도 거부한다. 기존 빈 이름·중복 이름을 유지하는 방향/범위/repeat 편집은 허용한다. 중복인 현재 이름을 동일하게 보내도 이름 변경으로 취급하지 않는다. 기존 빈 이름을 명시적으로 보내는 것은 거부하므로 생략/null을 사용한다. 겹치거나 동일한 범위를 허용하며 순서는 Aseprite가 정한다.

성공 응답은 `success`, 새 파일 `revision`, 저장·재열기 후 `tag` 한 항목이다. 범위 변경으로 `tag.tag_id`가 바뀔 수 있다. 후속 작업 전에 다시 조회한다. 기존 `create_tag`/`delete_tag`의 필수 입력·schema·성공 payload·이름 기반 선택 및 기존 3방향 생성 계약은 바꾸지 않는다. 새 도구의 정책을 기존 도구 전체에 소급 적용하지 않는다.

## 보호와 API 근거

조회는 공통 읽기 잠금 아래 Lua 실행 전후 bytes 해시를 비교하며 save·transaction·history 기록을 하지 않는다. 수정은 기존 source/history lock → private staging → bound 복사본 revision 확인 → Lua transaction → 저장·재열기 검증 → 원본 변경 감지 → atomic publish 경로를 사용한다. 원본 파일에서 잠금을 잡은 뒤 복사본 해시를 검사하므로 조회 후 대상을 바꿔치기한 요청을 조용히 적용하지 않는다. 성공한 편집은 opt-in history 및 snapshot/undo 대상이다. 실패·취소·변경 없음은 원본과 성공 이력을 바꾸지 않는다.

입력 범위/형식 검사는 `invalid_arguments`, stale은 `file_changed`, 없는 파일은 `not_found`, 문서 frame 범위/태그 부재/중복 이름/unchanged 등 Lua 의미 검사는 `lua_error`다. SDK schema 오류는 기존 protocol 경로다. 기존 request ID·redaction·capability 검사 계약을 유지한다. `repeats`를 늘려도 프레임이나 픽셀을 복제하지 않는다.

태그 객체를 삭제·재생성하지 않고 공식 setter로 변경하므로 color/data/properties를 유지한다. range setter가 태그를 재정렬하고 반대 endpoint를 clamp하므로 객체 참조를 유지하며 두 끝점을 설정한다. 저장 후 전체 태그 목록의 공개 속성과 data를 재검사한다. 레이어·cel·pixel·duration·native sharing에는 쓰기를 수행하지 않는다. 그룹 내부의 잠긴/숨긴 레이어가 있어도 태그는 문서 전역 속성이므로 편집할 수 있다.

근거: [공식 Tag API](https://www.aseprite.org/api/tag/), [v1.3.18.3 Tag Lua setters](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/app/script/tag_class.cpp), [native 정렬](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/doc/tags.cpp), [range 재정렬](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/doc/tag.cpp), [repeats 상한](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/doc/tag.h), [native 파일 규격](https://github.com/aseprite/aseprite/blob/v1.3.18.3/docs/ase-file-specs.md).

revision은 파일 내용 token이며 영구 identity나 서버 보관 snapshot이 아니다. 비협력 writer의 ABA·최종 검사/rename 사이 경쟁·전원 장애 durability는 [FILE_PROTECTION](FILE_PROTECTION.md) 및 [CEL_PROPERTIES](CEL_PROPERTIES.md)의 한계를 따른다. 조회는 전체 태그를 반환하며 페이지/응답 byte 상한은 없다. 대형 문서 로딩·긴 이름의 비용은 기본 timeout 30초에 포함된다. color/data/properties 편집, 영구 ID, timeline insert/delete의 새로운 semantics, GUI 선택 상태, tilemap/reference 교차 보존 검증은 이번 범위 밖이다. 기존 frame 도구의 삽입/삭제 계약을 강화했다고 주장하지 않는다.

## 검증

검증 대상 테스트:

- `TestTagsSavedModesAndUndo`: RGB/grayscale/indexed 4-frame 저장·재열기, native 순서·중복/빈 이름·overlap, injection 모양 이름의 안전한 왕복, 모든 direction, repeats 0/65535와 omission/null, 양방향 범위 이동·single frame, tag metadata 및 중첩/잠긴/숨긴 계층·cel 좌표/opacity/zIndex/link·duration 보존. 별도 프로세스에서 모든 frame을 렌더링하여 편집 전후 픽셀 배열을 비교하고 undo 및 수동 snapshot restore의 bytes와 재열기 결과 검사.
- `TestTagsRejectionsSchemaStaleAndOrder`: invalid/empty/negative/0/overflow/fraction/type/schema·중복 rename·missing tag·unchanged 거부, 원본 bytes/inode/mtime/mode 및 성공 이력 불변. 단일 endpoint·동일 범위 순서와 태그 생성/삭제/range·frame 삽입/삭제·metadata 변경 후 stale 거부.
- `TestTagsConcurrentRevision`: 같은 revision의 두 편집 중 한 번만 성공·이력 한 건.
- `TestTagsFailureAfterSave`: 실제 Aseprite 저장 후 Lua 오류·취소·외부 변경·snapshot quota 실패, 원본/외부 bytes·snapshot 개수 및 성공 이력 보존.
- `TestTagsCanonicalSaveAndReadOnly`: read-only 조회·history 저장소 미생성, missing 파일, native symlink 보존 및 비-native canonical 저장 이름 거부.
- `TestTagsNameBoundaryAndNumericOrder`: 12개 태그의 숫자 순번과 65535-byte UTF-8 이름의 실제 저장·재열기.
- `TestTagPropertiesValidation`, `TestTagsDiagnostics`: validation, 실제 server 등록/성공 request ID·오류 코드·redaction. 기존 animation/structure/cel/history/file protection은 관련 회귀 범위.

실행 결과와 최종 셀프리뷰는 아래 기록을 따른다. 테스트 이름은 검증 범위이며 모든 기능 조합의 검증을 의미하지 않는다.

## 통합과 plugin 후속

이 브랜치의 `animation.go`, `file_protection.go`, server 등록 수 테스트, README 한/영, CAPABILITIES/CHANGELOG/NEXT_STEPS/ROADMAP은 다른 Top4 PR과 충돌할 수 있다. 통합 시 미병합 코드를 임의로 가져오지 말고 등록 이름의 **합집합**으로 도구 수와 MCP inventory ID를 조정해야 한다. 다른 브랜치에서 동일한 MCP-059 등의 문서 추적 ID를 사용하면 통합 시 중복 없이 배정한다. export 구현은 기존 저장 태그를 직접 읽어 선택할 수 있으며 이 도구/응답에 의존하지 않는다.

pixel-plugin 저장소·pin·번들은 수정하지 않는다. Top1–3 develop 병합 후 한 번의 동기화에서 MCP pin/번들, 도구 inventory에 `get_sprite_tags`, `set_tag_properties`를 추가하고 animator skill에 조회→revision+tag_id→수정→재조회 흐름, `pingpong_reverse`, repeats 0과 omission 차이, duplicate/empty-name 정책을 반영하는 것을 권장한다. 응답의 `revision`, `tag.tag_id`와 native 순서를 보존하고 이름을 영구 ID로 사용하지 않는다. Top4 export 병합 후 exporter skill/번들은 별도 동기화한다. 새 도구 2개 추가 외에 기존 성공 payload/필수 입력 호환성 변화는 없다.

### 2026-10-04 실행 결과

- Linux amd64 Docker `pixel-tag-editing`, image `pixel-mcp-ci:latest` (`b0b99e1a1e34`), Go 1.25.14 / 실제 Aseprite 1.3.18.3-dev / API 41. container 내부 `/workspace`·`/tmp`, 전용 `/tmp/tag-config.json`, temp `/tmp/pixel-tag-editing`, timeout 30초. 다른 컨테이너 부하를 조회하고 `--cpus 2`, `GOMAXPROCS=2`, `-p 1`, 전체 suite `-parallel 1`로 부하를 제한했다. 이 컨테이너의 build→vet→race/coverage→integration은 순차 실행했다.
- 최종 `go build -p 1 -o /tmp/pixel-mcp ./cmd/pixel-mcp`, `go vet -p 1 ./...`, CLI health 통과.
- 최종 `go test -p 1 -parallel 1 -count=1 -race -cover ./...` 전체 통과. pkg/aseprite 82.4%, pkg/tools 62.4%, pkg/server 90.7%. 기본 suite statement coverage이며 기능 조합 완성률이 아니다.
- 최종 `go test -p 1 -parallel 1 -count=1 -tags=integration ./...` 전체 통과. pkg/tools 504.267초. 새 태그 테스트 및 기존 도구/파일 보호/history/snapshot/export 회귀를 포함하며 실패 테스트를 필터로 제외하지 않았다. 기본 suite의 조건부 skip 전수 감사는 하지 않았다.
- 앞선 Linux 태그 집중 `-race -tags=integration`은 tools 56.175초, server 2.500초 통과. 이후 테스트만 보강한 snapshot restore/실패 snapshot 불변/최대 이름 검사는 위 최종 전체 integration에 포함한다. production 구현은 동일하다.
- macOS arm64 / Go 1.25.0 / 실제 Aseprite 1.3.18.2-arm64 / API 41. `/tmp/pixel-tag-editing-evidence/macos-config.json`, 별도 temp/cache, canonical `TMPDIR=/private/tmp`로 build/health 및 최종 관련 회귀 통과. `-p 1 -parallel 1 -count=1 -tags=integration ./pkg/tools ./pkg/server -run 'TestTags|TestTagProperties|TestSnapshotToolsRegistered|TestCelProperties|TestSpriteStructure|TestStructureInput|TestIntegration_(CreateTag|DeleteTag|SetFrameDuration|DuplicateFrame)' -v`: tools 95.969초, server 1.876초. 선택된 검사에 skip 없음.
- 이 작업에서 별도 PBT/fuzz suite는 발견하지 못했다. 고정 native fixture·경계값·원본/렌더링/metadata 불변식으로 검증했다. 지원 하한 전체 suite는 정책상 반복하지 않았으며 [1.3.17.2 tag setters](https://github.com/aseprite/aseprite/blob/v1.3.17.2/src/app/script/tag_class.cpp)와 [enum 등록](https://github.com/aseprite/aseprite/blob/v1.3.17.2/src/app/script/engine.cpp)을 소스로 대조했다. macOS 전체/race와 tilemap/reference 교차 검증, 클라이언트 UI는 이번 실행 범위 밖이다.
- Go 포맷·`git diff --check`·문서 상대 링크·한/영 inventory 60개 대조 통과. 최종 8개 변경 Go 파일의 host/container SHA-256 일치를 확인했다. 실행 로그와 manifest는 로컬 `/tmp/pixel-tag-editing-evidence/`에 보존했다(임시 증거이며 영구 배포 artifact가 아님).

환경/개발 중 실패 기록:

- 최초 제한 sandbox에서 GitHub DNS/Docker socket 접근이 거부됐고 native `--batch --version`이 exit 134로 중단됐다. 사용자 인계에 따르면 `_RegisterApplication`/NSApplication 단계의 windowserver/launchservices mach-lookup deny이며 편집 코드 실행 전 환경 실패다. 승인된 환경 재개 후 동일 version probe·Docker·Git 원격 확인이 성공했다. 글로벌 설정을 변경하지 않았다. 초기 pure `pkg/config` 검사는 별도로 통과했다.
- 최초 테스트 helper가 bound scope 밖에서 `SpriteRevision`을 호출해 실패했다. 테스트 oracle을 원본 bytes의 독립 SHA-256 계산으로 고친 뒤 재검증했다. production guard를 제거하거나 우회하지 않았다.
- 확대 macOS 회귀의 한 실행은 capability probe 및 canonical `.gif` 거부 검사 중 각각 기본 30초 timeout으로 실패했다. 다른 native 검사가 동시 실행 중이었으나 인과를 확정하지 않는다. 다른 관련 native 검사의 종료 후 같은 후보·같은 timeout으로 전체 선택을 재실행하여 위 최종 통과를 확인했다. 기능 오류로 단정하거나 timeout을 늘리지 않았다.

### 최종 셀프리뷰

`convergent-code-review` backend lens, review-and-repair 허용 범위에서 검토했다. baseline/initial HEAD/merge-base는 모두 `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732`다. 기준부터 최종 working diff 21개 파일(Go 8개·문서 13개)을 범위로 고정했다. 후보 manifest SHA-256은 `25f54427b82bd39ddcdf3db8edbce541193d595c37094e4b5df41c78f56a0653`이며 전체 파일 동일성을 검증한 뒤 이 실행 결과·리뷰 보고만 추가했다.

- Reviewed: 새 Go 입력/schema/handler·Lua 조회/수정, registration/read-only/history 제외 경로, 실제 MCP/API 회귀·진단·등록 검사, 관련 계약/현황/한영 inventory.
- Inspected for context: 기존 animation/create/delete 및 cel/structure handler·generator, common wrapper, Client.ExecuteLua/capability, revision·lock/staging/publish·history/snapshot, server diagnostics, fixture/helper, 공식 Tag setter/정렬/native 규격. 직접 계약 전파 경로로 한정했다.
- Changed during repair: 없음. Excluded: 다른 Top4 구현·미병합 branch, 범용 metadata/color 편집·GUI·timeline 변경 semantics, plugin 저장소·번들, 배포.
- 초기 discovery 1회, fresh-discovery 전체 범위 검토 1회, 총 review pass 2회. repair cycle·repair-diff·closure pass 0회(리뷰 후 관련 수정 없음). 최초/후속 P0/P1/P2/P3 지적 모두 0건. VERIFIED/ACCEPTED/OUT_OF_SCOPE/미해결 finding 각 0건, 재개방·수정 유발 결함 0건. 후보 선택 후 관련 runtime/contract/test 변경·인증 무효화 0회. 소요 시간은 별도 측정하지 않았다.
- 독립적인 fresh 검토는 이름이 아닌 순번 선택·재정렬·동일 범위·중복/빈 이름, explicit zero/null·native 저장 범위, Lua 문자열 경계, read-only/history, stale/동시/취소·symlink 저장형식, 저장 후 응답 revision 및 기존 API 호환성을 다시 추적했다. 통과 테스트만을 체크리스트로 사용하지 않았다.

Notes:

1. tag_id/revision은 내용이 같은 저장 문서에서만 유효하다. 비협력 writer/ABA/최종 검사 이후 경쟁과 전체 태그 응답의 비용·무페이지 제한은 위 계약을 따른다. 후속 검토·plugin 반영은 [NEXT_STEPS의 태그 후속 TODO](NEXT_STEPS.md#태그-후속-todo)에서 추적한다.
2. macOS 전체/race 및 명시된 교차 조합은 미실행이며 동시 native 실행 중 timeout 관찰을 기록했다. 필요한 CI/운영 병렬 실행 범위는 별도 검증해야 한다.
3. 다른 Top4 PR과 공통 파일/count/문서 ID 충돌 가능성이 있다. plugin pin/번들/skill 동기화와 독립 외부 리뷰, develop/main 병합·배포는 수행하지 않았다.

판정: **PASS_WITH_NOTES**. 최종 후보의 필수 검증과 fresh 전체 검토를 완료했고 범위 내 미해결 차단 결함이 없어 추가 수정 cycle을 진행하지 않는다. 작업 브랜치·워크트리는 미병합이므로 정리하지 않는다.

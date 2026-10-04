# 레이어 속성 및 그룹 편집 (GAP-01)

작업: `[BE][FEATURE] 레이어 속성 및 그룹 편집` · `feature/be-layer-properties`.
기준: `origin/develop` `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732` (PR #29/#30/#31 병합 포함).
현재 후보는 Unreleased이며 develop/main 병합·태그·배포는 수행하지 않는다. 타 Top4 작업과 plugin 저장소는 이 범위에 포함하지 않는다.

## 호출 계약

`get_sprite_structure`에서 받은 구조 ID와 `revision`을 사용한다. 아래 도구는 모두 `sprite_path`와 `expected_revision`이 필수다. 파일은 요청 경로와 symlink 해석 후 실제 저장 경로 모두 `.ase`/`.aseprite`여야 한다(대소문자 허용). revision은 파일 전체 bytes의 64자리 소문자 SHA-256이다.

| 도구 | 필수 추가 입력 | 선택 입력·의미 |
| --- | --- | --- |
| `set_layer_properties` | `layer_id` | `name`, `visible`, `editable`, `opacity`, `blend_mode` 중 최소 하나. 생략/null은 유지. 빈 이름, false, opacity 0은 실제 값 |
| `move_layer` | `layer_id`, `parent_id`, `stack_index` | 레이어 또는 그룹 전체 subtree를 이동/재정렬. 부모와 대상 ID 모두 **편집 전 revision**의 ID |
| `create_layer_group` | `name`, `parent_id`, `stack_index` | 빈 그룹 생성. 기존 레이어를 자동으로 넣지 않음. 이름은 빈 문자열·중복 허용 |

`layer_id`는 양의 1-based 형제 index 경로(`2/1`)이며 선행 0이나 이름 경로를 받지 않는다. `parent_id=""`는 sprite 루트다. parent/index는 생략하지 않는다. `stack_index`는 부모 안의 bottom-to-top **최종** 순서(1-based)다. 같은 부모 내 이동은 원본 제거 후 재삽입한 결과 index이며 1…기존 형제 수, 다른 부모 이동·생성은 1…목적지 기존 자식 수+1이다. 0·음수·소수·범위 밖은 거부한다. 다른 부모로 이동한 뒤 비워진 그룹은 남는다.

```json
{"sprite_path":"/absolute/art.aseprite","expected_revision":"<조회 revision>","layer_id":"2/1","name":"Ink","visible":true,"opacity":0}
```

```json
{"sprite_path":"/absolute/art.aseprite","expected_revision":"<새 조회 revision>","layer_id":"2/1","parent_id":"","stack_index":1}
```

```json
{"sprite_path":"/absolute/art.aseprite","expected_revision":"<새 조회 revision>","name":"Effects","parent_id":"2","stack_index":1}
```

속성 값이 모두 같거나 같은 부모/같은 순서로 이동하는 요청은 no-op 오류다. 이름은 native string의 65535 UTF-8 byte 상한을 적용하고 NUL을 거부한다. Lua 입력은 JSON 직렬화 후 기존 EscapeString을 통과한다.

## 속성·잠금·지원 경계

- 일반 raster와 group을 지원한다. root/nested·중복 이름은 구조 ID로 선택한다. 대상 tilemap/reference/background 및 내부 `__mcp_clipboard__` 레이어는 거부한다. 해당 예약 이름으로 생성/rename하지 않는다.
- `visible`/`editable`은 **로컬** 속성이다. 숨김 대상·숨김 조상 아래의 편집과 숨김 그룹으로 이동은 허용한다. `effective_visible`/`effective_editable`은 모든 조상과 자신의 로컬 값의 AND다. GUI 선택·timeline collapse와 무관하다.
- 잠긴 조상 아래의 편집은 거부한다. 로컬 잠금 대상은 **`editable=true` 단독 요청만** 허용한다. 다른 변경은 먼저 unlock한 후 새로운 revision으로 요청한다. unlocked 그룹을 잠그면 자손의 로컬 editable은 그대로이고 effective 값만 바뀐다.
- 이동은 대상·조상·모든 자손과 목적지·목적지 조상이 unlocked여야 한다. 이동 subtree에 특수/clipboard 레이어가 있으면 거부한다. self-parent, 자기 자손으로 이동, raster를 부모로 지정하는 요청을 거부한다. native background 아래 삽입도 거부한다.
- opacity는 raster의 0…255 정수다. group opacity/blend는 공식 Layer API에서 nil이며 지원하지 않는다. group을 raster로 바꾸거나 flatten으로 우회하지 않는다.
- blend는 아래 19개의 소문자 이름을 공식 `BlendMode` 상수에 대응한다. `src`, 임의 숫자, 별칭, 알 수 없는 값은 거부한다. SRC는 이미지 그리기용이며 native layer 모드가 아니다.

```text
normal multiply screen overlay darken lighten color_dodge color_burn
hard_light soft_light difference exclusion hsl_hue hsl_saturation
hsl_color hsl_luminosity addition subtract divide
```

RGB/grayscale/indexed의 저장 속성·계층을 대상으로 하며 모드 변환이나 indexed 픽셀 재매핑을 수행하지 않는다. blend 합성 결과는 Aseprite의 색상 모드별 native renderer를 따른다. 세 모드의 multiply/screen/normal RGB 렌더 픽셀과 opacity 0/255를 검증한다. indexed 원본의 palette/픽셀 index를 변경하거나 저장 모드를 자동 변환하지 않는다. 19모드 전체 저장 roundtrip과 모든 색상·팔레트 조합의 렌더링 검증은 구분한다.

그룹 삭제·recursive destructive operation, tilemap/tileset·reference/background 편집, cel 픽셀/unlink, tag 수정, export 범위, 범용 metadata 편집은 추가하지 않는다. 기존 `add_layer`/`delete_layer`/`delete_frame`/`flatten_layers` 입력과 동작·마지막 layer/frame 보호를 유지하며 새로운 ID 기반 삭제 계약으로 확대하지 않는다. 새 도구는 layer/frame을 삭제하지 않는다.

근거: [공식 Layer API](https://www.aseprite.org/api/layer/), [BlendMode](https://www.aseprite.org/api/blendmode), [native 파일 규격](https://github.com/aseprite/aseprite/blob/v1.3.18.3/docs/ase-file-specs.md).

## 응답·ID 무효화·보호

세 도구의 성공 응답은 `success`, 저장된 파일의 새 `revision`, 저장/재열기한 대상 `layer`다. layer에는 `layer_id`, `parent_id`, `name`, `kind`, `stack_index`, 로컬 `visible`/`editable`, effective 값이 있다. raster는 `opacity`와 `blend_mode`를 포함하고 group에서는 두 필드를 생략한다. 조회 `get_sprite_structure.layers[]`에도 optional `blend_mode`만 추가하며 기존 필수 입력·성공 필드를 유지한다.

반환 ID는 **편집 후** 대상 ID다. 이동/재정렬/그룹 삽입은 대상 외 형제·자손·목적지 ID도 바꿀 수 있다. 모든 변경 후 구조를 다시 조회하고 페이지 간 revision도 비교한다. 응답은 전체 ID migration map을 제공하지 않는다. 이름이 같고 기존 ID가 다른 레이어를 가리키더라도 stale revision은 거부한다.

공통 source/history lock → staging → 실제 bound bytes의 revision 검사 → Lua transaction → native setter 결과 확인 → 저장/재열기한 전체 계층·로컬/effective 속성·cel 위치/opacity/z-index/픽셀/크기·native 공유 확인 → 원본 변경 감지·atomic publish를 사용한다. 검증 실패·취소는 발행하지 않는다. 전체 계층과 cel 픽셀을 검사하므로 큰 문서의 메모리/실행 비용은 증가하며 기본 timeout 30초를 유지한다.

기존 사용자 sprite/layer/cel/tag data·extension properties, frame duration, palette/transparent index와 descendants를 변경하는 명령을 사용하지 않는다. 테스트에서 독립 재열기해 보존을 검사한다. 계층·합성 속성 변경으로 렌더링이 달라지는 것은 요청된 결과다. native image 공유를 분리하거나 픽셀을 복사해 이동하지 않는다.

revision은 내용 token이고 영구 UUID/단조 sequence가 아니다. byte 단위 undo는 과거와 같은 revision을 복구할 수 있다. 비협력 writer의 ABA 및 최종 검사와 rename 사이 경쟁 등 [CEL_PROPERTIES](CEL_PROPERTIES.md#오래된-구조동시-변경-방어)/[FILE_PROTECTION](FILE_PROTECTION.md)의 보장 경계를 그대로 유지한다.

MCP 형식 오류는 schema/RPC, Go 값 검사는 `invalid_arguments`, stale은 `file_changed`, 누락 파일은 `not_found`, 대상/잠금/cycle/no-op/저장 검증 오류는 기존 `lua_error`다. 공개 오류는 경로·사용자 이름을 노출하지 않고 기존 request ID/redaction/timeout/cancel 계약을 따른다.

`enable_history=true`이면 성공한 세 도구를 각 이름으로 기록한다. 실패·no-op은 history를 추가하지 않는다. 전후 snapshot/undo와 용량·TTL 계약은 [HISTORY](HISTORY.md)를 따른다. Lua transaction 자체는 호출 간 undo나 crash-atomic save가 아니다.

## Plugin 반영과 병합 충돌

이 후보는 기준 develop 58개에 도구 3개를 더해 61개다. 타 Top4 도구는 포함하지 않는다. 다른 PR 병합 후 숫자를 합산하지 말고 실제 등록 이름 집합을 다시 계산해야 한다.

- Top1~3 develop 병합 후 MCP commit pin/번들/스킬 동기화에 세 도구와 optional structure `blend_mode`, `LayerEditOutput` 계약을 포함한다. Top4 export 이후 별도 plugin 동기화 계획은 유지한다.
- `pixel-art-creator`에 조회→revision→속성/그룹 편집→구조 재조회 흐름, 잠금 해제 단독 호출, final sibling index, group opacity/blend 및 destructive 삭제 제외를 반영한다. 새 mutation을 기존 이름 기반 add/delete/flatten으로 대체하지 않도록 설명한다.
- `pixel-art-animator`에는 레이어 이동 후 cel의 structural ID/image_ref를 다시 조회해야 한다는 참조를 추가하는 것을 권장한다. cel/tag/export 기능 자체는 해당 독립 작업 계약을 따른다.
- README 한/영, CAPABILITIES의 MCP ID 번호·도구 수, NEXT_STEPS, ROADMAP, CHANGELOG, server 등록 수 테스트와 structure 계약 파일은 병합 충돌 가능성이 있다. 이 작업은 canvas 등록을 사용하며 animation.go를 수정하지 않는다. 타 미병합 브랜치를 가져오지 않는다.

## 검증·리뷰 기록

최종 후보 검증 기록이다. 과거 PR의 검증 완료 기록과 구분한다.

- macOS arm64 / Go 1.25.0 / 실제 Aseprite 1.3.18.2-arm64 / API 41: 로컬 CLI build·health 통과. 전용 config/temp, timeout 30초, `GOMAXPROCS=2`, `-p 1 -parallel 1 -count=1 -tags=integration`. 새 `TestLayer*`, 기존 cel/structure/input/회귀 matrix/nested drawing, 등록 및 공개 오류 검사를 실행해 tools 155.959초, server 6.509초 통과. 검증 명령의 전체 필터는 아래와 같다.
- Linux amd64 Docker: 전용 `pixel-layer-properties`, image `pixel-mcp-ci:latest` (`b0b99e1a1e34`), container 내부 `/workspace`·`/tmp`를 사용한다. Go 1.25.14 / Aseprite 1.3.18.3-dev / API 41. 전용 `/tmp/layer-config.json`, temp `/tmp/pixel-layer-properties`, timeout 30초. 타 작업의 Docker 전체 검사가 종료되고 CPU 사용률이 내려간 뒤 시작했다. `GOMAXPROCS=2`, `-p 1`, 테스트 `-parallel 1 -count=1`을 사용해 build → vet → health → 전체 race/coverage → 전체 integration 순서로 실행했다.
- Linux build/vet/health와 `go test -race -cover ./...`: 통과. 기본 suite statement coverage는 aseprite 82.6%, tools 62.5%, server 90.7%다. 모든 기능 조합의 완성률이나 integration coverage를 뜻하지 않는다. `go test -tags=integration ./...` 전체도 통과했다(tools 420.852초). 테스트를 skip하거나 실패 case를 필터로 제외한 실행이 아니다. 기본 suite의 기존 환경 조건부 skip을 별도 전수 감사한 것은 아니다.
- 신규 검사는 세 색상 모드의 nested 이동·그룹 생성/재정렬, 명시적 0/false/빈 이름·생략/null·schema 오류, 중복 이름·Lua injection 문자열, local/effective 잠금·가시성, 19개 blend 저장 및 대표 RGB 렌더, native links·metadata·tag/frame 보존, 세 도구의 history/snapshot/byte-exact undo, stale/동시 요청·missing·cancel/실패·canonical 확장자 보호를 포함한다. 세 형제의 모든 출발/도착 index 조합을 독립 metadata identity로 대조한다.
- 별도 PBT/fuzz suite는 저장소에서 찾지 못했다. 고정 fixture와 순서 조합 열거로 관계·픽셀·bytes 불변식을 검사했다. macOS 전체/race, 모든 blend×색상×팔레트 조합과 대형 파일 성능 검증은 수행하지 않았다. 지원 하한 전체 suite는 정책에 따라 반복하지 않았다.
- 실제 빌드된 MCP `tools/list`의 61개 고유 이름·신규 필수 schema를 한·영 README/CAPABILITIES와 대조했다. 변경 Go 9개 파일의 host/container SHA-256 일치, gofmt, diff 공백과 변경 문서의 로컬 상대 링크를 확인했다.

```sh
PIXEL_MCP_CONFIG=/private/tmp/pixel-layer-properties-evidence/macos-config.json \
GOMAXPROCS=2 go test -p 1 -parallel 1 -count=1 -tags=integration ./pkg/tools ./pkg/server \
-run 'TestLayer|TestCelProperties|TestSpriteStructure|TestStructureInput|TestRegressionMatrix|TestDrawPixelsLayer|TestSnapshotToolsRegistered|TestToolDiagnostics|TestIntegration_(AddLayer|DeleteLayer|DeleteFrame|Flatten)' -v
```

초기 실행 이력: 제한 sandbox에서 Docker 소켓/GitHub 접근이 거부되고 macOS version probe가 exit 134로 실패했다. 사용자 환경 복구 뒤 version/Docker/GitHub를 재검사해 통과했으며 이 이력은 편집 로직 결함이 아니다. 초기 회귀 작성 중 history 최신순 가정과 도구별 schema 입력, indexed fixture의 transparent index 변경 뒤 남은 초기 cel을 수정했다. 해당 실패를 숨기거나 skip하지 않고 수정한 전체 관련 macOS 검사를 다시 통과했다. 초기 병렬 작업 중 macOS 조회 두 건의 지연/실패도 관찰했으며 최종 관련 검사에서 재현되지 않았다. timeout을 늘리지 않았고 이를 제품 결함이나 확정된 원인으로 단정하지 않는다.

임시 로컬 증거: `/private/tmp/pixel-layer-properties-evidence/`의 `linux-final.log`, `macos-final.log`, `tool-inventory.json`, `go-manifest.txt`. 영구 배포 artifact는 아니다.


### 최종 셀프리뷰

`convergent-code-review` backend lens로 기준·HEAD·merge-base `96d8fa4a4f93a8b895eb1c924b1c9fa74f51d732`부터 이 후보의 작업 트리 diff 전체(22개 파일: Go 9, 문서 13)를 검토했다. 초기 discovery 1회, 최종 후보 fresh-discovery 전체 범위 검토 1회, 총 review pass 2회. 초기/신규 P0/P1/P2/P3 지적 0건, repair cycle/repair-diff/closure 0회(리뷰 지적에 따른 수정 없음), 재개방·수정 유발 결함·accepted/out-of-scope finding·미해결 지적 0건이다. 차단 결함이 없어 추가 수정 cycle을 진행하지 않는다. 리뷰 시간은 별도 측정하지 않았다.

- **Reviewed:** 새 Go 입력/schema/handler·Lua 생성기·saved-state 검증·integration/unit/server 회귀, 기존 structure의 additive blend 출력·canvas 등록·등록 수 및 13개 문서.
- **Inspected for context:** 직접 연결된 common/file protection/history/SpriteRevision/ExecuteLua, 기존 structure/cel generator와 fixture, server diagnostics/등록, 공식 Layer/BlendMode/native file 계약.
- **Changed during repair:** 없음. 초기 개발 중 fixture/schema 기대 보정은 위 실행 이력과 구분한다.
- **Excluded:** 타 Top4 구현과 branch, plugin 저장소, 기존 add/delete/flatten 확장, 배포·병합.

필수 입력·nil/0/false → 타입/범위 검사 → 안전한 JSON/Lua 직렬화 → staging revision → source/destination identity와 cycle/order/lock → transaction 및 saved hierarchy/cel-sharing → atomic publish/history/undo → 공개 오류/응답과 문서 계약을 대조했다. fresh 검토는 passing test 목록을 대신 따르지 않고 stale 대상 오인, source 제거로 바뀐 destination ID, linked 관계 손실, 그룹 속성·잠금 우회, native save encoder와 실패 후 발행 가능성을 다시 추적했다. 새 결함을 발견하지 못했다.

최종 코드/계약 manifest SHA-256: `d43771a55ace14a385a8f879edfab869d2a2ae7ba6cfe34880b86763b238ef62` (`candidate-manifest.txt`, 본 검증 보고 부분 제외). Go manifest SHA-256: `6df010a1728c4ef5c3df00a3e4cada8870af979724a4503dc9fc2864ae3b957a`. 후보 고정 뒤 runtime/contract/test 변경 및 certification 무효화는 0회이며 실행 결과·리뷰 보고만 추가했다.

Notes:

- 전체 계층·cel bytes 검사 비용과 비협력 writer의 ABA/최종 검사 경쟁은 위 계약의 한계다. 대형 파일 성능·모든 blend 조합, macOS 전체/race는 위 검증 범위 밖이다.
- 독립 외부 리뷰·develop/main 병합·태그/배포는 하지 않는다. 타 Top4 PR 통합 시 공통 문서/MCP ID/등록 수 충돌을 재확인해야 하며 plugin 반영은 위 후속 목록을 따른다.

판정: **PASS_WITH_NOTES**. 필수 검증이 통과했고 동일 후보의 전체 범위 재검토에서 차단 결함이 없다. 이 작업의 PR 병합 전에는 브랜치·워크트리를 정리하지 않는다.

# 상세 구조 및 cel 조회 (GAP-02 첫 단계)

작업: `[BE][FEATURE] 상세 구조 및 cel 조회`, `feature/be-sprite-structure`.
기준: `origin/develop` `2ec6c79b184ba04d2c0cfcbf9ff49ed7475cadd2` (PR #27/#28 병합 포함).
이 브랜치에서 구현한 Unreleased 조회 기능이며 develop 병합·배포와 구분한다.

## 호환성과 입력

별도 `get_sprite_structure`를 등록한다. 기존 `get_sprite_info`의 필수 입력과 응답은 그대로다.

```json
{"sprite_path":"/absolute/sprite.aseprite","layer_id":"2/1","frame_start":1,"frame_end":2,"layer_offset":0,"page_size":50}
```

`sprite_path`만 필수다. 모든 선택 숫자는 정수이며 생략/0은 기본값이다.

| 입력 | 계약 |
| --- | --- |
| `layer_id` | 생략/빈 문자열은 전체 계층. 지정하면 해당 레이어만 조회하며 자식은 자동 포함하지 않음. 양의 1-based 형제 순서 경로 (`2/1`), 선행 0·이름 경로 거부 |
| `layer_offset` | 필터 결과의 0-based 레이어 offset, 기본 0. 전체 개수와 같으면 빈 페이지, 초과/음수 거부 |
| `page_size` | 기본 50, 최대 100. 음수 또는 100 초과 거부 |
| `frame_start` | 1-based, 기본 1. 문서 frame 수 초과/음수 거부 |
| `frame_end` | inclusive. 기본 `min(frame_count, frame_start+99)`. 역순, 음수, 문서 범위 초과, 100-frame span 초과 거부 |

입력 형식 오류는 기존 MCP schema/RPC 오류 경로, Go validation은 `invalid_arguments`, 존재하지 않는 구조 ID 또는 문서 범위 밖 frame/offset은 기존 `lua_error` 경로다. 누락 파일은 `not_found`. request ID·redaction·timeout/cancel·capability 검사와 per-file lock은 공통 경로를 사용한다.

## 응답과 식별자 유효범위

문서의 `width`, `height`, `color_mode`, `frame_count`, 전체 재귀 `layer_count`, 필터 후 `matching_layer_count`, 실제 `frame_start`/`frame_end`, `layers`를 반환한다. 마지막 페이지에는 `next_layer_offset`이 없고, 마지막 frame 범위에는 `next_frame_start`가 없다. 다음 레이어 페이지는 같은 필터·frame 범위와 반환 offset을 사용한다. 다음 frame 범위는 같은 레이어 offset·페이지 크기와 반환 start를 사용하고 end를 생략한다.

`layers`는 각 형제의 bottom-to-top 순서를 따른 depth-first preorder다. 숨김·잠금 레이어 및 내부 clipboard 레이어도 포함한다.

| 레이어 필드 | 의미 |
| --- | --- |
| `layer_id`, `parent_id` | 형제 index를 `/`로 연결한 구조 경로, 루트 parent는 빈 문자열 |
| `name`, `name_path` | 원본 이름과 루트부터의 이름 배열. 중복·슬래시·따옴표·줄바꿈도 안전하게 표현. 이름 경로는 유일 ID가 아님 |
| `kind`, `stack_index` | `group`/`raster`/`tilemap`, 부모 안의 1-based 순서 |
| `visible`, `editable` | 해당 레이어의 로컬 속성 (`editable=false`는 잠금) |
| `effective_visible`, `effective_editable` | 모든 조상의 로컬 속성을 AND한 계층 결과. GUI 선택이나 timeline collapse는 반영하지 않음 |
| `opacity` | API가 제공하는 0–255 값. API가 nil인 group은 생략 |
| `cels` | group은 `[]`. 나머지는 선택 frame마다 하나의 존재/부재 레코드 |

`layer_id`는 런타임 메모리 ID나 파일에 저장하는 영구 UUID가 아니다. 같은 계층/형제 순서의 저장 파일을 재열면 동일하다. 이름만 바뀌어도 index 경로는 유지되지만 삽입·삭제·이동·재정렬 후에는 다른 레이어를 가리킬 수 있다. frame 번호도 삽입/삭제로 바뀐다. 파일을 편집하면 다시 조회해야 한다. 페이지 사이 snapshot을 고정하는 token은 없으므로 외부 변경이 있으면 처음부터 재조회한다. 기존 mutation tool은 이 신규 `layer_id` 입력을 수용하지 않는다.

| cel 필드 | 의미 |
| --- | --- |
| `frame_number`, `exists` | 1-based frame 및 cel 존재 여부. 부재는 이 두 필드만 포함하며 픽셀 투명 여부와 별개 |
| `x`, `y`, `width`, `height` | cel image 원점의 sprite 절대 좌표와 image 크기. 음수·canvas 밖 좌표를 그대로 반환 |
| `opacity`, `z_index` | 해당 cel의 값, 0–255 및 native z-index |
| `image_ref`, `linked_cel_count` | 같은 native image를 공유하는 cel들의 대표 참조 (`layer_id@frame_number`)와 문서 전체 공유 개수. 1은 독립 이미지 |

대표 참조는 가장 이른 frame을 가리키며 동일 frame이면 계층 순서상 최초 레이어를 선택한다. 링크 집합과 대표는 페이지/필터 밖 cel도 포함한다. 여러 페이지의 같은 `image_ref`로 관계를 재구성할 수 있으며 개수와 anchor를 반환해 거대한 멤버 목록 반복을 피한다. 내부 `Image.id`는 한 batch에서 공유 판별에만 쓰고 응답에 노출하지 않는다. 동일 픽셀의 독립 이미지는 서로 다른 참조다. 참조는 문서가 변하지 않는 동안만 유효하다.

## API 결정 및 제한

근거: [Layer API](https://www.aseprite.org/api/layer/), [Cel API](https://www.aseprite.org/api/cel/), [Image API](https://www.aseprite.org/api/image/), [native 파일 규격](https://github.com/aseprite/aseprite/blob/v1.3.18.3/docs/ase-file-specs.md).

실제 API/decoder 확인: linked cel의 위치·opacity는 Lua setter로 함께 바뀐다. linked 파일 헤더에 다른 위치/opacity가 있으면 [1.3.18.3 decoder](https://github.com/aseprite/aseprite/blob/v1.3.18.3/src/dio/aseprite_decoder.cpp#L817)는 독립 이미지로 복사한다. z-index가 다른 cel은 공유를 유지한다. 따라서 원본 chunk의 link 표기보다 **재열기 후 native image 공유**를 계약으로 삼는다. 원본 파일을 수정해 이를 정규화하지 않는다.

한 호출은 최대 100 레이어 × 100 frame, 픽셀 데이터나 사용자 data/properties를 반환하지 않는다. 이름과 계층 깊이에 따라 응답 byte 수는 달라지며 byte 상한은 없다. 필요하면 페이지와 frame span을 줄인다. Aseprite는 파일 전체를 로드하고 공유 집계는 전체 레이어/실제 cel을 순회하므로 페이지는 로딩/집계 비용을 없애지 않는다. 기본 timeout 30초를 유지한다. 대형 파일의 snapshot pagination·streaming은 이번 범위 밖이다.

RGB/grayscale/indexed의 group/raster를 검증한다. tilemap은 kind로 구분하나 편집·tileset/pixel 해석은 제공하지 않으며 이 작업의 검증 범위 밖이다. tilemap image 크기는 tile 단위일 수 있으므로 raster pixel 크기로 해석하지 않는다. reference layer의 확대 bounds가 아닌 native image 크기와 위치를 보고한다.

선택·GUI 상태에 의존하지 않는 batch 조회이며 save/transaction/edit/history 기록을 수행하지 않는다. 파일 접근용 잠금·임시 Lua script는 공통 실행 기반의 부수 파일이다. layer/cel 편집, unlink, 태그 수정, 색상 모드 변환, R4 export 추가는 미구현이다. GAP-02 전체 완료가 아니다.

## 검증과 셀프리뷰

검증 결과는 아래 최종 기록을 따른다. `TestSpriteStructureSavedModes`는 세 모드의 저장·재열기, 중복 이름 계층, 숨김/잠금, empty cel, non-zero/negative 위치, multi-frame, native link와 서로 다른 z-index, 픽셀이 같은 독립 이미지를 검사한다. Go가 조회 결과를 독립 상수와 비교한다. 성공·실패·페이지 반복 후 source bytes/inode/권한/mtime 및 사용자 metadata·history 불변을 검사한다.

`TestSpriteStructureLegacyLinkDecodesAsCopy`는 공식 native cel header에 따라 generated fixture의 linked 위치/opacity를 바꾼 후 실제 decoder에서 copy가 됨을 확인한다. 테스트 fixture 생성에만 raw header 수정을 사용하며 production은 파일을 파싱하거나 쓰지 않는다. `TestSpriteStructureFrameWindowAndSchema`는 102-frame 기본 범위/continuation과 필수입력·schema 오류를 검사한다. server diagnostics 회귀는 성공 request ID와 invalid_arguments/lua_error/not_found를 확인한다.

2026-10-04 최종 검증:

- 전용 Docker `pixel-sprite-structure`, image `pixel-mcp-ci:latest` (`b0b99e1a1e34`), container 내부 `/workspace`·`/tmp`. Linux amd64 / Go 1.25.14 / 실제 CLI Aseprite 1.3.18.3-dev / API 41. 별도 `/tmp/structure-config.json`, temp_dir `/tmp/pixel-structure`, timeout 30초. 다른 FIX 컨테이너의 전체 suite가 끝나고 부하가 내려간 것을 확인한 뒤 전체 검사를 순차 실행했다.
- `GOMAXPROCS=2 go build -p 1 ./...`, `go vet -p 1 ./...`: 통과. 최초 기본 병렬도 build/vet 연속 실행은 Go 도구의 SIGSEGV로 종료됐으며 Rosetta 실행 환경에서 병렬도를 줄인 재실행은 통과했다. 원인을 코드 결함으로 단정하지 않는다.
- `GOMAXPROCS=2 PIXEL_MCP_CONFIG=/tmp/structure-config.json go test -p 1 -count=1 -race -cover ./...`: 통과. pkg/aseprite 83.0%, pkg/tools 63.9%, pkg/server 90.7%. statement coverage이며 기능 조합 완성률이 아니다.
- 같은 환경의 `go test -p 1 -count=1 -tags=integration ./...`: 전체 통과. pkg/tools 400.760초. 새 조회 테스트 및 기존 get_sprite_info·drawing·history·snapshot·export 회귀를 포함한다. 필터로 기존 실패를 제외하지 않았다. 기본 suite의 모든 조건부 skip을 별도 전수 감사한 것은 아니다.
- macOS arm64 / 실제 Aseprite 1.3.18.2-arm64: Docker Go에서 darwin/arm64로 cross-compile한 테스트 바이너리를 호스트에서 실행했다. 별도 config/temp 사용. `TestSpriteStructureSavedModes` 세 모드 7.00초, 101-layer/102-frame 페이지·schema 1.08초, legacy link→copy 세 모드 1.98초, input validation 통과. server의 실제 성공·오류/request ID 계약 1.18초 및 등록 수 57 검사 통과.
- 변경 Go 파일 `gofmt -l` 출력 없음, `git diff --check` 통과. README 한/영 inventory와 CAPABILITIES의 실제 도구 57개를 대조했고 로컬 문서 링크를 검사했다. 테스트 컨테이너와 최종 워크트리의 변경 Go 파일 SHA-256 일치를 확인했다.
- 별도 PBT/fuzz suite는 저장소에서 발견하지 못했다. 이번에는 고정 native fixture의 관계·페이지·source bytes/metadata 불변식을 실제 API로 검사했다. macOS 전체/race, 앱 UI 및 tilemap/reference layer 교차 검증은 미실행이다. 지원 하한 전체 suite는 기존 이력 및 NEXT_STEPS 정책에 따라 반복하지 않았다.
- 로그: 로컬 `/tmp/pixel-mcp-sprite-structure-evidence/`의 `linux-build.log`, `linux-vet.log`, `linux-race.log`, `linux-integration.log`, `macos-tools.log`, `macos-server.log`. 임시 로컬 증거이며 영구 배포 artifact는 아니다.

셀프리뷰: `convergent-code-review` backend lens를 사용해 기준 `2ec6c79`부터 최종 관련 diff(Go 8개, 문서 8개)를 검토했다. 입력/default/schema → safe Lua 직렬화 → 읽기 전용 파일 보호/history 제외 → 재열기 native 공유 → JSON 배열/필터/페이지 → 기존 등록/응답 호환성을 대조했다. 문맥 확인은 직접 연결된 `common.go`, `Client.ExecuteLua`, 파일 보호 helper, server 등록·diagnostics, 기존 canvas/link/history 테스트와 공식 API/decoder에 한정했다. RM-FIX-01 구현과 layer/cel 편집·unlink·태그 수정·모드 변환·R4 export·GUI 확장은 제외했다.

초기 discovery 1회, fresh-discovery 전체 범위 재검토 1회, 총 review pass 2회. 초기/신규 발견 P0/P1/P2/P3 0건, repair cycle·repair-diff·closure pass 0회(리뷰 후 수정 없음), 재개방·미해결 0건. 최종 후보 선택 후 관련 runtime/contract/test 변경·certification 무효화 0회. 검증 결과를 기록하는 보고 문장만 추가했다. 코드/계약 후보 manifest SHA-256은 `b60e0a476cb0dabfea6b86fafb2ef340f54a155da6c0a7e5636d3b69000e0096`이며 로컬 evidence 디렉터리에 manifest를 보존했다. 리뷰 소요 시간은 별도 측정하지 않았다.

Notes:

- 구조 ID/frame/image 참조는 변경되지 않은 저장 문서에 한정한다. 페이지 간 snapshot 고정, 영구 ID 및 기존 mutation 도구의 ID 수용은 후속이다.
- native load/전체 공유 집계 비용과 이름/깊이별 byte 규모는 페이지 제한만으로 제거되지 않는다. tilemap/reference 및 앱 UI·macOS 전체/race는 위 검증 범위 밖이다.
- 독립 외부 리뷰와 develop/main 병합·배포는 수행하지 않았다. RM-FIX-01과 공통 CHANGELOG/CAPABILITIES/NEXT_STEPS/ROADMAP/REGRESSION_MATRIX 문서가 겹칠 수 있어 병합 순서에 따라 상태 문구를 조정해야 한다. 미병합 FIX 코드를 사용하지 않는다.

판정: **PASS_WITH_NOTES**. 범위 내 차단 결함과 미해결 지적이 없고 최종 후보 검증·전체 범위 재검토를 완료해 추가 수정 cycle을 진행하지 않는다. GAP-02는 조회 부분만 완료했으며 편집/unlink는 후속이다. feature 브랜치·워크트리는 미병합이므로 정리하지 않는다.

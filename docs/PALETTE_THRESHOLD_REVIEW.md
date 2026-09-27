# [BE][FIX] 팔레트·민감도·디더링 경계 수정

검증일: 2026-09-27. `convergent-code-review`의 review-and-repair 방식으로
확정 버그 3건과 사용자가 추가 승인한 DITHER-01/02를 처리했다. 팔레트 추출의
기존 조사 노트에서도 초기 수렴 오류를 재현해 같은 소비 경로 안에서 수정했다.

## 기준과 범위

- Worktree: `/Users/keumheesung/orca/workspaces/pixel-mcp/be-fix-palette-thresholds`.
- Branch: `fix/be-palette-thresholds`.
- Base / initial HEAD / merge-base: `8d9bdde15c463b1c8227cfdb3a7d8cf65bfac966`
  (origin/develop, PR #20 포함). 시작 시 작업 트리는 깨끗했다.
- 다른 작업의 feature/shared-dry-run(PR #21), 원본 main, 플러그인 워크트리는 수정하지 않았다.
- Reviewed: palette resize, reference/AA threshold 처리, dither 생성, 참조 palette clustering,
  직접 소비 테스트와 CAPABILITIES/NEXT_STEPS/ROADMAP/CHANGELOG. 총 16개 파일.
- Context: 기존 palette/draw helpers, MCP SDK schema, file-protection wrapper,
  기존 antialiasing/quantization/analysis 회귀와 로컬 플러그인 테스트 클라이언트.
- 플러그인 배포용 pin/5개 번들은 이 PR에서 바꾸지 않는다. MCP 병합 후 별도 반영이 필요하다.

## 수정과 실제 전후 결과

전: 고정 MCP 8d9bdde 내장 바이너리. 후: 이 브랜치 소스로 빌드한 실제 stdio MCP.
같은 8×8 fixture/인자를 별도 파일에 적용하고, 독립 Aseprite 검사기로 저장 픽셀을 읽었다.

| 항목 | 수정 전 | 수정 후 |
|---|---|---|
| indexed 팔레트 4→2 (미사용 끝 항목 제거) | 빨강32·파랑32 → 빨강32·파랑0 (파랑 영역 투명화) | 빨강32·파랑32 유지 |
| analyze_reference edge_threshold=0/1/30 | 검출된 칸 0/12/0 | 12/12/0 |
| 반투명 계단(alpha64) AA threshold=0/1/63/64/128/255 | 제안 5/5/5/5/5/5 | 5/5/5/0/0/0 |
| Floyd density=.25/.5/.75 | color2 픽셀 32/32/32 | 16/32/49 |
| checkerboard density=.25/.5/.75 | color2 픽셀 32/32/64 | 16/32/48 |
| dots density=.25/.5/.75 | color2 픽셀 56/56/64 | 28/56/60 |
| Bayer 4×4 대조군 .25/.5/.75 | color2 픽셀 16/32/48 | 16/32/48 (그대로) |
| 빨강63·초록1 참조 palette, 10회 | 이 실행에서 빨강100%만 반환 | 매회 빨강98.4375%·초록1.5625%, 결과 동일 |

Floyd/checkerboard/dots/Bayer의 **density=.5 픽셀 해시는 전후 동일**했다.
새 integration 테스트는 13개 Floyd/texture 패턴의 중간값 변화와 0/1 endpoints,
1×8·8×1·1×1·3×5 영역의 결정성·영역 밖 보존을 확인한다. 16패턴 기존 endpoint,
indexed endpoint와 Bayer 작은 양수 회귀도 유지했다.

### 정확한 동작 정의와 호환성

- SetPalette와 AddPaletteColor는 resize 전 transparentColor를 보관하고 색 설정 후 복구한다.
  기존 투명 픽셀과 마스크를 유지하며, 마스크가 palette 범위 밖에 있어도 임의로 opaque index로 이동시키지 않는다.
  일반 palette index 제거의 임의 remap/색상 보존 기능을 새로 약속하지는 않는다.
- Reference edge_threshold는 생략/null만30, 명시적0은0. AA threshold는 생략/null만128.
  두 입력은 nullable integer가 됐고 기존 JSON 숫자 입력·응답은 유지된다. Go 직접 호출자의
  `AnalyzeReferenceInput.EdgeThreshold` / `SuggestAntialiasingInput.Threshold`는 *int로 바뀐다.
- AA는 **기존 계단 후보**의 두 색을 premultiplied RGBA 8-bit로 변환해 최대 채널 차이가
  threshold보다 큰 경우만 제안/적용한다. 빈 픽셀은 투명, 숨은 RGB의 차이는 무시한다.
  opaque/transparent의 대비는255이며, 이것은 기존 AA 패턴·혼색 알고리즘 전체를 재설계한 것은 아니다.
- Floyd는 기존 가로 gradient의 평균 혼합량을 density로 조절한다. .5는 폭>1에서 기존 결과를 유지하고,
  폭1은 중앙 mixture와 1차원 carry를 사용한다. 이 폭1의 기존 all-color1 동작은 의도적으로 변경된다.
- Texture는 원래 0/1 문양의 각 그룹에 결정적 순위를 부여해 중간 threshold를 세분화한다.
  .5 문양 호환성을 우선하므로 dots의 .5는 원래대로 color2가56/64다. density를 모든 패턴의
  정확한 전역 색 비율로 해석하면 안 된다. 별도 exact-ratio API는 추가하지 않는다.
- Palette clustering은 결정적 farthest-point seed와 첫 assignment=-1을 사용한다.
  첫 평균 계산 전 잘못 수렴하지 않으며 서로 다른 sampled 색을 우선 seed로 선택한다.
  기존 k개 결과 항목 수를 유지하므로 고유색이 적으면 중복/usage0 항목은 남을 수 있다.
  subsampling/근사 추출이 이미지의 모든 소수색을 항상 보존한다는 보장은 아니다.
- 도구50개와 모든 output schema가 이전과 동일하다. 변경된 input/description은
  analyze_reference, suggest_antialiasing, draw_with_dither 세 도구뿐이다. 기존 warnings도 유지한다.

## 검증

환경: macOS arm64, Aseprite 1.3.18.2-arm64 / API41, Go1.25.0.
실제 config/temp 파일은 이 워크트리의 ignored tmp/review 및 tmp/go-tmp 안에 둔다.

| 검증 | 결과 |
|---|---|
| 수정 전 TestAudit 회귀 | palette mask, explicit0, AA threshold, Floyd/texture 중간값, schema와 k-means 초기화/소수색 테스트가 실패 |
| 수정 후 TestAudit + TestDitherDensity | 전체 통과 (경계, 잘못된 인자의 원본 보존, auto_apply, null/default 포함) |
| go vet ./... | 통과 |
| go test -count=1 -race -cover ./... | 환경 보정 후 통과 |
| go test -count=1 -json -tags=integration ./... | 1132 test/subtest 통과, 실패·개별skip0 |
| darwin amd64/arm64, linux amd64/arm64, windows amd64 | 5종 cross-build 통과 |
| 실제 old/new stdio MCP 비교 | 위 수치와 .5 기본 결과의 동일 해시 확인 |
| 실제 tools/list | 50 tools, 모든 output schema 동일, 예상한 세 input/description만 변경 |
| gofmt / git diff --check | 통과 |

첫 race/coverage 실행은 coverage 보조 프로세스가 PATH에서 go를 찾지 못해 일부 no-test 패키지에서 실패했다.
같은 Go executable 디렉터리를 PATH에 추가해 **동일 소스의 전체 race/coverage**를 재실행했고 통과했다.
제품 실패를 성공으로 숨긴 것이 아니며 초기 로그 race.log와 재실행 race-fixed-env.log를 구분한다.

원자료: tmp/review/red.log, focused.log, integration.log, race-fixed-env.log,
comparison/results.json 및 calls.json, contract-comparison.json, builds.json.
이전 플러그인의 1094개 테스트 결과를 이번 실행으로 사용하지 않았다.

재현 명령(실제 config를 먼저 작성):

```bash
export PATH="/path/to/go/bin:$PATH"
export PIXEL_MCP_CONFIG="/absolute/path/to/config.json"
export TMPDIR="/absolute/canonical/temp/path"
go test -count=1 -tags=integration ./pkg/aseprite ./pkg/tools -run 'TestAudit|TestDitherDensity'
go vet ./...
go test -count=1 -race -cover ./...
go test -count=1 -json -tags=integration ./...
```

## Convergent review ledger

| ID | 우선도 | 원인·위치 | 처리 / 검증 |
|---|---|---|---|
| BE-P1-001 (MCP-AUDIT-01) | P1 | lua_palette.go: resize의 mask clamp 노출 | SetPalette/AddPaletteColor에서 mask 복구. 0/3/255와 실제 픽셀 비교 VERIFIED |
| BE-P2-002 (MCP-AUDIT-02) | P2 | analysis.go: zero-value를 default로 처리 | *int로 생략/null/0 구분. 0/1/30 실제 edge 대조 VERIFIED |
| BE-P2-003 (MCP-AUDIT-03) | P2 | antialiasing.go: threshold 무시 | 후보 대비 필터, 경계/원본 보존/auto_apply 검사 VERIFIED |
| BE-P2-004 (기존 palette 조사 노트) | P2 | palette.go: assignment 초기0 조기 수렴 및 무작위 중복 seed | k=1 평균 red→green, 소수색·10회 결정성 회귀 VERIFIED |
| BE-P2-005 | P2 | 중간 문서 갱신 뒤 남아 있던 DITHER 미구현/BUG-02 병합 대기 문구 | CAPABILITIES/NEXT_STEPS/ROADMAP 상태 일치, 최종 검색·링크 확인 VERIFIED |

DITHER-01/02는 이미 문서화된 제한을 사용자가 명시적으로 개선 요청한 범위이며,
신규 회귀 버그로 세지 않는다. 기능 구현과 원래0.5 호환성·중간값·작은 영역 테스트를 완료했다.

리뷰 활동: 최초 전체 검토1, 수정 cycle1, repair-diff1, closure1, fresh 전체 검토2 = 총5회.
최초 P1 1/P2 3; 문서 상태 불일치 P2 1이 추가돼 수정했다. 재오픈0, accepted0, 미해결0,
repair로 발생한 runtime 결함0, VERIFIED5. 초기 임시 snapshot은 문서 상태 보정 후1회 교체했고,
최종 runtime/test candidate는 전체 검증 후 변하지 않았다. 최종 문서까지 새로 대조했다.

| 단계 | 열린 P0 | 열린 P1 | 신규 blocker | 확인된 P1 해결 |
|---|---:|---:|---:|---:|
| Initial | 0 | 1 | 1 | 0 |
| Cycle/closure | 0 | 0 | 0 | 1 |
| Fresh final | 0 | 0 | 0 | 1 |

정지 이유: 합의된 범위의 실제 실패를 수정했고, closure·fresh review 및 전체 검증에서 새 blocker가 없었다.
독립 외부 리뷰나 merge는 수행하지 않았다. 시간은 측정하지 않았다.

## 남은 경계 (미수정 버그 목록과 구분)

- 다른 OS는 cross-build만 했으며 Windows native·최소 버전 설치·앱 GUI 검증은 이번에 반복하지 않았다.
- 일반 drawing의 색상 모드·cel geometry 전체 재설계, AA 패턴 범위/혼색 개선, palette 결과 cardinality 변경,
  exact-ratio 새 모드, dry-run PR #21, snapshot/undo/crash 복구는 제외했다.
- 원래3건의 runtime 버그와 요청한 DITHER 중간값 미반영은 이 브랜치에서 수정·검증됐다.
  아직 develop/main 병합·배포나 플러그인 내장 binary 갱신을 뜻하지 않는다.

Verdict: **PASS_WITH_NOTES**.

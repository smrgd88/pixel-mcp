# RM-FIX-01 — 그룹 내부 draw_pixels 대상 조회

작업: `[BE][FIX] 그룹 내부 draw_pixels 대상 조회 수정` · 2026-10-04
브랜치: `fix/be-nested-draw-pixels`
워크트리: `/Users/keumheesung/orca/workspaces/pixel-mcp/be-fix-nested-draw-pixels`
기준: `origin/develop` `2ec6c79b184ba04d2c0cfcbf9ff49ed7475cadd2` (PR #27/#28 병합 포함). 이 FIX는 PR #29로 develop `2ced64ca0da429764cb39f7ced37f349917a4448`에 병합했으며 Unreleased다. 아래 검증·리뷰·독립 작업 기록은 당시 이력이다.

## 문제와 수정 계약

기존 DrawPixels는 `ipairs(spr.layers)`만 검사하여 `visible-group/paint`의 `layer_name="paint"`를 `Layer not found: paint`로 거부했다. 기준 코드의 기존 4-case matrix를 실행해 이 오류와 원본 bytes 보존을 재현했다. 그룹 자식 `Layer.layers`를 재귀 탐색하여 저장된 계층의 raster 대상을 찾는다. 다른 drawing generator는 변경하지 않는다.

- `layer_name`은 대소문자를 구별하는 **정확한 literal 이름**이다. 루트/중첩 모두 계층 전체에서 한 번만 등장해야 한다. slash는 경로 구분자가 아니며 기존 이름의 일부로 취급한다. UUID·경로·새 selector를 추가하지 않는다.
- 중복 leaf 이름(루트/다른 그룹/동일 그룹)이나 group/raster 이름 충돌은 `Ambiguous layer name` Lua 오류로 거부한다. 루트 우선·첫 일치·visible 우선 같은 암묵 선택은 없다. 호출 전에 사용자/Aseprite에서 고유 이름으로 변경해야 한다. 구조 조회 기능은 이 FIX의 의존성이 아니다.
- 기존 group 경로는 `Sprite:newCel()`의 `unexpected kind of layer`로 실패함을 같은 API 호출 순서의 실제 batch probe로 확인했다. 고유 group 또는 tilemap 대상은 `Target layer must be a raster layer`로 거부한다. missing은 기존 `Layer not found`다. 모든 대상 검사는 transaction/저장 전에 수행한다.
- hidden·locked raster와 숨김/잠금 조상 안의 raster도 직접 pixel 편집을 허용한다. visibility/editability 플래그와 계층은 유지한다. 기준 코드에서 root hidden/locked 저장·재열기 성공을 실제로 확인했고 동일 정책을 nested에 적용했다. GUI 도구 잠금 정책을 새로 강제하지 않는다.
- 유효 frame에 cel이 없으면 기존처럼 cel을 생성한다. 좌표는 sprite 절대 좌표이며 canvas 밖은 거부한다. 유효 cel은 필요한 만큼 확장하고 native linked image 공유를 유지한다. 기존 색상·palette/transparent index 경로를 재사용한다.
- 필수 입력·`pixels_drawn` 성공 payload·warnings·공개 오류 envelope는 유지한다. 중복 이름을 첫 일치에 쓰던 입력은 이제 실패한다. 위 오류 문구는 Lua/비래핑 fixture 진단이며 번들 CLI의 공개 메시지는 기존 redacted `lua_error` 계약을 따른다. invalid frame<1은 기존 `invalid_arguments`다.

공식 근거: [Layer.layers/isGroup/isImage/isTilemap/visibility/editability](https://www.aseprite.org/api/layer/), [Sprite.layers/newCel/saveAs](https://www.aseprite.org/api/sprite/), [Cel.image/position](https://www.aseprite.org/api/cel/). API 제공 사실과 실제 batch 저장 결과는 구분한다.

## 회귀 근거

- `TestRegressionMatrixLinkedDrawingExport`: RGB/grayscale/indexed(mask 0/3), positioned (5,3) 2×2 cel, frame 2 native link를 대상으로 정확한 단일 `(1,2,#FFFFFFFF)` 요청 후 추가 픽셀을 편집한다. 그룹 밖 이동 없이 저장/재열기 image identity·expanded 위치(1,2)/크기(6,3)·palette mask와 새 투명 영역·계층/숨김 레이어·문서 data/frame duration을 검사한다. 두 PNG의 모든 16×16 픽셀과 export 원본 bytes 불변도 검사한다. nested 좌표 실패는 진단 문구와 bytes 보존을 assert한다.
- `TestDrawPixelsLayerPolicy`: root 및 2-level nested에서 정상/hidden/locked/숨김·잠금 조상/no-cel/literal 특수 이름을 저장·재열기 검사한다. slash/quote/backslash/newline/Lua 코드 모양 이름이 실행되지 않고 정확히 해당 레이어에만 적용되는지 확인한다.
- `TestDrawPixelsLayerRejectionPreservesSource`: root/nested/sibling 중복·group/leaf 충돌·missing·root/nested group·tilemap·frame 0/없는 frame·음수 x/경계 밖 y를 오류 기대와 원본 bytes로 검사한다.
- 기존 animation integration의 3개 fixture는 기본 `Layer 1`에 같은 이름을 추가해 첫 일치를 편집하던 입력이었다. 중복 거부 계약에 맞춰 추가/편집/link 대상 이름을 고유 `paint`로 보정했다. 애니메이션 검증과 production animation 코드는 유지한다.
- 기존 `TestIntegration_DrawPixels*`와 generator unit 테스트는 루트 동작·좌표·native link 비회귀 근거다. 새 무작위 PBT framework는 도입하지 않으며 구체 fixture의 전 픽셀·bytes 불변식을 검사한다.

## 독립성 및 제한

GAP-02는 별도 `feature/be-sprite-structure` / `be-feature-sprite-structure` 작업이다. 그 워크트리·코드·문서는 수정하지 않는다. 이 FIX는 조회 도구 없이 완결되며 새 도구/공유 selector helper를 만들지 않는다. 두 PR은 코드 의존성이 없지만 NEXT_STEPS/ROADMAP/CAPABILITIES/CHANGELOG 같은 공통 문서에서 충돌할 수 있으므로 통합 시 두 상태를 함께 보존해야 한다.

다른 drawing tools의 nested 이름 처리, path/UUID 편집 선택, tilemap pixel 편집, 프레임별 palette·서로 다른 linked 위치·blend/부분 alpha의 전체 교차 matrix는 확대하지 않는다. 광범위 matrix 전체 완료·배포를 주장하지 않는다. 지원 하한 전체 재실행은 기존 완료 이력과 NEXT_STEPS 정책에 따라 제외한다. macOS는 관련 회귀만 실행하며 전체/race·앱 UI는 이 작업 검증 범위에 포함하지 않는다.

## 실행 및 셀프리뷰 (FIX 작업 당시 기록)

환경: 전용 `pixel-rm-fix-01` 컨테이너, Linux amd64 / Go 1.25.14 / Aseprite `1.3.18.3-dev`, API 41 (`--health` success=true). 소스를 `/workspace` 내부 filesystem에 복사했으며 host bind mount를 쓰지 않았다. 전용 `/tmp/rm-fix-config.json`, temp `/tmp/pixel-rm-fix-01`, timeout=30을 사용했다. 사용자 config는 수정하지 않았다. 검증 시작 시 다른 컨테이너는 유휴 상태였으며 이후 다른 작업의 Go 컴파일 부하를 관찰했다. 이 세션의 Aseprite 실행은 순차 수행했다.

- Linux `go build ./...`, `go vet ./...`, `go test -count=1 -race -cover ./...`: 통과. pkg/aseprite 83.0%, pkg/tools 64.0%, pkg/server 90.7%. 이후 변경은 integration-tagged fixture 이름과 보고서뿐이므로 이 기본-suite 결과는 최종 runtime 코드에 해당한다.
- 첫 전체 `go test -count=1 -tags=integration ./...`: pkg/tools 336.511초, 위 중복 fixture 3건 실패. 실패를 skip하지 않고 고유 이름으로 보정했다.
- 보정 후 Linux `go test -count=1 -tags=integration ./pkg/tools -run 'TestIntegration_DuplicateFrame_AtEnd|TestIntegration_LinkCel|TestRegressionMatrix|TestDrawPixelsLayer' -v`: 통과, 23.171초.
- macOS arm64 / 설치된 Aseprite `1.3.18.2-arm64`: Docker Go로 `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go test -c -tags=integration -o /tmp/rm-fix-tools-darwin.test ./pkg/tools` 후 호스트에서 전용 config를 선택해 실행. 필터 `TestRegressionMatrix|TestDrawPixelsLayer|TestIntegration_DrawPixels|TestIntegration_LinkCel|TestIntegration_DuplicateFrame_AtEnd`, 최상위 24개 그룹 통과. 4-case nested matrix 7.24초. macOS 전체/race를 뜻하지 않는다.
- health 및 기존 group API 거부 probe 통과. 변경 Go 파일 gofmt와 문서 상대 링크·`git diff --check` 통과. 검증한 컨테이너 소스와 호스트의 해당 Go 파일 bytes 동일성을 확인했다.
- 로그: `/tmp/pixel-rm-fix-01-evidence/`의 baseline/root-policy-baseline/focused-linux/linux-build-vet-race/linux-integration/linux-repair/macos/health-group 로그. 임시 경로이므로 영구 CI artifact는 아니다.

- 최종 Linux `go test -count=1 -tags=integration ./...`: 전체 통과. pkg/tools 415.522초. 로그 `linux-integration-final.log`. 첫 실패 이후 보정된 최종 fixture를 포함한 재실행이다.

### 최초 후보와 셀프리뷰 (222bc44 작성 시점)

리뷰 기준 HEAD/merge-base: `2ec6c79b184ba04d2c0cfcbf9ff49ed7475cadd2`. 후보는 이 기준 + staged 12-file 변경이며, 보고용 최종 결과 추가 전 tree는 `ebb5d1e9f76c3e5f1982605f06683b34805a98ae`다. runtime/test 6개 파일을 경로순으로 `path + NUL + bytes` 연결한 SHA-256은 `891cab5ef176f4e9295448964bd184c1dea1abb98085cb466fa91a7ae0bd1c17`이다. 후보 선택 후 runtime·test·계약 변경 및 certification 무효화는 0회다. 이후 추가한 실행 결과·리뷰 수치는 보고용 기록이다.

- Reviewed: 기준부터 최종 후보까지 production 2개(Lua generator/input 설명), test 4개(generator unit, matrix, layer policy/rejection, 기존 animation fixture), 문서 6개. 총 12개 파일, 작업 커밋 전 staged diff.
- Inspected for context: 직접 drawing handler, client 실행/오류 분류, EscapeString, file-protection wrapper, export/behavior fixture 및 palette helper. 다른 drawing generator는 문맥만 확인했다.
- Changed during repair: DrawPixels 계층 탐색·타입/모호성 검사, schema 설명, 관련 테스트와 문서. animation production 코드는 변경하지 않았다.
- Excluded: GAP-02 작업 워크트리/코드, 다른 drawing 도구 전체 리팩터링, 병합·배포, 전체 client/OS/색상 matrix.

| finding | 우선도 | 증거·처리 | 최종 상태 |
| --- | --- | --- | --- |
| RM-FIX-01 | P2 | 기준 matrix의 nested 조회 거부 → 성공 요청·재열기 identity/좌표·두 PNG 전 픽셀 검사 | VERIFIED |
| NESTED-P1-001 | P1 (필수 검증 차단) | 변경된 중복 거부 정책이 기존 animation fixture 3개에 노출되어 전체 suite 실패. 고유 paint 이름으로 입력 보정, 기존 assert 유지, Linux 관련·전체 및 macOS 회귀 | VERIFIED |

초기 discovery 1회, mutation cycle 2회(구현 1회, invalid fixture 보정 1회), repair diff review 2회, closure 1회(두 번째 repair diff review와 같은 pass), fresh-discovery 전체 범위 review 1회: 서로 다른 review pass 총 4회. 초기 P2 1건; cycle 1 후 새 P1 1건(계약 변경으로 노출된 테스트 입력); cycle 2/closure/fresh 신규 0건. repair 유발 검증 차단 1건, 재개방 0건, VERIFIED 2건, 미해결 0건. P0은 전 단계 0건, P1은 초기 0 → cycle 1 후 1 → cycle 2/closure/fresh 0이다.

closure는 보정된 모든 호출의 이름 일치와 애니메이션 assert 유지, 그룹 재귀/모호성 조기 거부, 실패 원본 보호 및 저장된 링크 결과를 확인했다. fresh-discovery는 이름 literal/계층/타입, public error 경계, no-cel/좌표 변환, palette/linked 공유, 저장·출력과 문서 계약을 전체 diff에서 다시 대조했다. 새 merge blocker는 없으며 마지막 runtime/test 변경 후 필수 검증이 통과했으므로 추가 mutation을 종료한다.

Notes:

1. macOS 전체/race와 앱 UI, 광범위 교차 matrix는 미검증이다. 지원 하한 전체 반복은 기존 완료 이력과 정책에 따라 제외했다. 별도 무작위 PBT suite는 없다.
2. GAP-02는 코드 의존성 없이 별도 PR이나 공통 문서 충돌 가능성이 있다. 중복 이름 선택용 path/UUID 입력은 추가하지 않았으며 호출 전 고유 이름이 필요하다.
3. 독립 외부 리뷰·원격 PR CI·develop/main 병합은 이 로컬 셀프리뷰와 별개다. 이번 작업은 커밋/PR 생성까지이며 미병합 branch/worktree는 정리하지 않는다.

판정: **PASS_WITH_NOTES**.


### Tilemap 거부 회귀 보강 및 재리뷰 (2026-10-04)

후속 권고였던 tilemap 분기 검증 공백을 해소했다. `TestDrawPixelsLayerRejectionPreservesSource/tilemap`은 공식 [NewLayer 명령](https://github.com/aseprite/api/blob/main/api/command/NewLayer.md)의 `tilemap=true`, `ask=false`로 실제 native fixture를 생성·저장한다. 별도 프로세스로 재열어 대상의 `isImage == true`, `isTilemap == true`, tileset 존재를 먼저 확인한다. 유효 frame/좌표/색상 요청이 정확한 raster 대상 오류로 거부되고 원본 bytes가 동일한지 검사한다.

- 분기 검증: 컨테이너 사본에서만 `or layer.isTilemap`을 제거하자 요청이 성공하여 새 테스트의 `IsError` assertion이 실패했다. 따라서 그룹/non-image 거부 검사에 우연히 기대는 fixture가 아니다. 원래 production 소스를 즉시 복구하고 워크트리와 byte 동일성을 확인한 뒤 아래 최종 검증을 수행했다.
- Linux Go 1.25.14/Aseprite 1.3.18.3-dev: `go vet -tags=integration ./pkg/tools` 통과. `go test -count=1 -tags=integration ./pkg/aseprite ./pkg/tools -run 'TestLuaGenerator_DrawPixels|TestDrawPixelsLayer|TestRegressionMatrix|TestIntegration_DrawPixels|TestIntegration_LinkCel|TestIntegration_DuplicateFrame_AtEnd'` 통과 (aseprite 0.350초, tools 30.496초).
- macOS arm64/Aseprite 1.3.18.2-arm64: 최종 소스로 테스트 바이너리를 재빌드한 뒤 `TestDrawPixelsLayerRejectionPreservesSource|TestRegressionMatrix` 필터 통과. 신규 tilemap 거부·원본 보존과 기존 RGB/grayscale/indexed native link matrix를 포함한다.
- 로그: `/tmp/pixel-rm-fix-01-evidence/{tilemap-mutation,tilemap-final-linux,tilemap-final-macos}.log`. production 변경 없이 integration fixture와 본 보고서만 보강했으므로 로컬 전체 build/race/coverage/integration·macOS 전체를 반복하지 않았다. 직전 `222bc44`의 로컬 전체 검증과 원격 CI 성공은 앞선 코드의 이력이며, 새 커밋 CI와 구분한다. 하한 전체 반복 제외 정책도 유지한다.

후속 리뷰 시작 HEAD는 `222bc44`, 통합 기준은 `2ec6c79`다. 최종 후보는 두 파일의 후속 diff를 포함하며, 앞서 정의한 runtime/test 6개 파일 연결 SHA-256은 `e6c4cff87e703ce3e935a8357448e1eb2e8c703dd50642026f952d4a532b8298`이다. 최초 후보의 검증 기록은 역사로 유지하고, 이번 후보는 아래 closure/fresh review로 다시 판정했다.

리뷰 범위는 기준 `2ec6c79`부터 전체 12개 변경 파일 및 직접 handler/client/escaping/file-protection 문맥이다. 이번 수정은 테스트 1개·문서 1개이며, GAP-02와 다른 drawing/animation production 코드는 제외했다. 초기 확인 1회, mutation cycle 1회, repair-diff/closure 검토 1회, 전체 범위 fresh-discovery 1회로 총 review pass 3회다. tilemap 검증 공백은 VERIFIED; 신규 P0/P1/P2/P3, 수정 유발 결함, 재개방 및 미해결 지적은 각각 0건이다. 최종 후보 선택 뒤 runtime/test/계약 변경·무효화는 0회다.

closure에서 저장 후 타입 전제, 정상 입력, 오류 문구 및 bytes 검사가 실제 tilemap 분기를 검증함을 확인했다. 전체 재리뷰에서 대상 이름의 유일성·타입 판별, 좌표/linked 공유, 오류 시 저장 보호와 문서 계약을 다시 대조했다. 필수 관련 검증이 통과하고 새 결함이 없어 추가 수정을 종료한다.

Notes는 앞선 macOS 전체/race·앱 UI 및 광범위 matrix 제한, GAP-02 공통 문서 충돌 가능성, 독립 외부 리뷰·병합 상태에 한정한다. tilemap 거부 fixture 부재는 더 이상 잔여가 아니다. 판정: **PASS_WITH_NOTES**.

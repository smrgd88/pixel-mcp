# 기능 로드맵

기준일: 2026-09-22 · 코드 기준: `develop`의 `a42c6242077c4681a9b66f44915f7aef8de33a6c`

[기능 지원 현황](CAPABILITIES.md) · [상세 실행 체크리스트](NEXT_STEPS.md) · [변경 이력](../CHANGELOG.md)

## 현재 위치

50개 MCP 도구가 등록되어 있다. 기본 드로잉·애니메이션·선택·팔레트·변형·export와 자체 분석 기능을 제공한다. `v0.1.0`의 47개에서 `v0.4.0`에 감색/자동 shading 2개, `v0.5.0`에 flatten 1개가 추가됐다.

포크의 config 격리, draw_pixels 좌표, native linked cel, indexed shading 수정은 develop에 반영됐지만 `v0.5.0`에는 없다. 신규 릴리스 번호나 배포 일자는 확정하지 않는다. R0–R5는 작업 단계이며 버전 번호가 아니다.

기능 상태의 기준은 CAPABILITIES, 작업 상세·검증 이력은 NEXT_STEPS, 우선순위·의존성은 이 문서에 둔다. 기능마다 문서를 따로 복제하지 않고 ID로 연결한다.

## 다음 작업 순서

최신 develop에서 재현한 알려진 동작 오류를 SAFE-02 dry-run보다 먼저 처리한다. 잘못된 실제 결과와 모호한 감색 계약을 preview에 그대로 복제하지 않기 위한 순서다. 재현 조건과 완료 조건은 [NEXT_STEPS](NEXT_STEPS.md#현재-우선-작업--알려진-동작-오류)에 둔다.

| 순서 | 작업 | 이유 |
| --- | --- | --- |
| 1 | BUG-04/05 감색 결과·옵션 계약 — PR #16 병합 완료 | 픽셀 remap·입력 모드 유지·indexed 투명 인덱스 분리. 상세 계약은 CAPABILITIES |
| 2 | BUG-01 density=0 보존 — 수정 브랜치, 병합 대기 | 명시적 사용자 입력이 다른 픽셀 결과를 만드는 오류 |
| 3 | BUG-03 다중 frame PNG — 다음 구현 작업 | 기존 export workflow와 실제 파일/응답 계약 복구; 필요한 출력 경로 처리만 정리 |
| 4 | BUG-02 참조 형식 지원 | 광고된 지원과 decoder 불일치. 독립 처리 가능하며 dry-run 자체의 기술적 선행 조건은 아님 |
| 5 | SAFE-02 dry-run | 올바른 편집 동작을 임시 복사본에서 preview |
| 6 | SAFE-03 → SAFE-04 | snapshot/restore 이후 history/undo |

파일 접근 선언 구조의 전체 리팩터링은 제안 상태이며 별도 선행 작업으로 확정하지 않는다. 필요 부분은 해당 수정/기능 작업에 포함할 수 있다. BE 수정은 원인별 branch/PR로 분리하는 것을 권장하며 플러그인 작업은 포함하지 않는다.

## 단계와 완료 조건

| 단계 | 목적·추적 ID | 주요 작업 | 완료 조건 | 선행 조건·현재 상태 |
| --- | --- | --- | --- | --- |
| R0 | 현황·릴리스 근거 정리, OPS-01 | 기능표/태그/Unreleased 동기화, CI Go·Aseprite 버전 artifact (로그 출력은 구현됨), draw_pixels upstream 제출 범위 결정 | 등록 50개와 기능표 일치; 버전 증거 보존; 제출 범위 기록 | 문서 초안 작성 완료, CI artifact·범위 결정 예정 |
| R1 | 실행 계약 확립, SAFE-01·GAP-10·OPS-03 | warnings, capability preflight, 오류 분류 및 응답 규칙 설계 | 위험 작업별 경고·기존 클라이언트 호환 검증; 미지원 버전/API를 변경 전에 거부 | warnings develop 병합 완료. capability PR #13 병합 완료; 하한 health·전체 통합 실검증 완료 (2026-09-22) |
| R2 | 원본 보호와 복구, SAFE-02–05·OPS-03 | path lock·atomic save 기반, 임시 복사본 dry-run, snapshot/restore, history/undo, request ID·redaction | 실패·취소·동시 요청 시 원본 보존; dry-run 원본 불변; snapshot 복원·history 일관성 회귀 통과 | R1 이후. undo는 snapshot 이후 |
| R3 | 편집·조회 빈 부분 해소, GAP-01–04 | 레이어 속성·그룹, 상세 구조 조회, cel 위치/opacity/unlink, 태그 조회·편집, 범용 색상 모드 전환 | 다중 레이어/프레임에서 정확한 대상 지정; 저장/재열기 검증; destructive 변경에 R1/R2 계약 적용 | R1/R2 기반 이후, 예정 |
| R4 | 게임·UI 자산 export, GAP-05–06 | 태그/레이어/프레임 범위 export, trim/extrude/개별 padding, slices/pivot/nine-slice | 출력 이미지와 JSON의 frame·bounds·pivot 일치; overwrite/실패 보호 | R3 상세 조회 기반, 예정 |
| R5 | 호환성·확장, GAP-07–09·GAP-11–12·OPS-02 | native launcher와 OS smoke, 색상/계층/client matrix 확대; tilemap/tileset·보정 필터·metadata·선택/brush/grid 확장 | 플랫폼별 실행 증거, 기능별 최소 API 및 batch 검증, 기존 도구 회귀 통과 | launcher/matrix는 독립 착수 가능. 확장 기능은 별도 계약·SPIKE 후 선정 |

R5의 확장 후보 전체를 하나의 릴리스에 묶지 않는다. GUI 플러그인·실시간 편집 세션(GAP-13)은 현재 batch 구조 밖이므로 별도 SPIKE에서 필요성과 구조를 결정한다.

## 안전성 기능의 의존 순서

SAFE-01은 PR #11로 develop에 병합했다. GAP-10은 PR #13, SAFE-05는 PR #14로 병합했다. 위 알려진 동작 오류를 먼저 처리한 뒤 SAFE-02 dry-run으로 진행한다. 실제 앱 UI 호환성 확인은 후속 검증으로 남긴다.

1. `[SHARED][FEATURE] 위험 작업 warning 반환` — SAFE-01, upstream #15. 공통 optional warnings와 감색·flatten·색상 모드 변경·보간 scale 조건부터 고정한다.
2. `[SHARED][FEATURE] Aseprite capability 사전 검사` — GAP-10. health 정보와 변경 전 지원 하한 검사를 연결한다.
3. `[SHARED][FEATURE] 파일 변경 보호 기반` — SAFE-05. 같은 파일의 동시 실행·timeout/cancel과 atomic 교체를 검증한다.
4. `[SHARED][FEATURE] 임시 복사본 dry-run` — SAFE-02, upstream #12. preview summary와 실제 결과의 일치를 검증한다.
5. `[SHARED][FEATURE] snapshot 및 restore` — SAFE-03, upstream #13. ID·metadata·복원 전 보호·TTL/용량/권한 정책을 구현한다.
6. `[SHARED][FEATURE] 작업 이력 및 undo` — SAFE-04, upstream #14. 성공한 변경만 기록하고 snapshot과 연결한다.
7. R3 → R4 기능 확장. OS launcher·matrix는 별도 OPS 작업으로 진행할 수 있다.

R0의 CI 버전 기록은 `[OPS][CHORE] CI 실행 버전 기록`으로 다음 릴리스 전 마무리한다. upstream #19 제출 범위 결정도 R0 잔여다. 이는 SAFE-01/GAP-10 기능 작업과 별개의 운영 잔여 항목이다.

Indexed primitive 공통화와 transparent index 불변식 확대 검증은 R3의 색상 모드 전환 전에 수행한다. 공통화 완료 여부는 auto shading 버그 수정 완료와 구분한다.

## 보류와 재개 조건

- upstream #16 indexed 검은 이미지: 보고된 Aseprite 1.3.15.5는 프로젝트 지원 하한 미만. 지원 버전에서 재현되거나 릴리스 검증에 필요할 때 재개한다.
- 새 기능의 공식 API 존재는 구현 가능성 근거다. 최소 지원 버전과 batch 동작을 확인하기 전에는 지원으로 승격하지 않는다.
- 날짜·버전 약속은 두지 않는다. 각 단계는 검증 근거로 완료하며 기능 범위에 따라 여러 릴리스로 나눌 수 있다.

## 버전과 릴리스 추적

현재 저장소의 CHANGELOG는 SemVer와 Keep a Changelog를 선언한다. 이번 작업에서는 버전을 올리거나 태그·Release를 발행하지 않는다.

- PR마다 CAPABILITIES의 상태/제한과 CHANGELOG의 Unreleased를 갱신한다.
- 호환 버그 수정은 PATCH, 호환 기능 추가는 MINOR 후보로 본다. 도구 삭제·필수 입력·응답 의미 변경과 지원 하한 상향은 호환성 변경으로 별도 검토한다.
- 현재 0.x 단계의 breaking-change 버전 규칙과 정식 릴리스 절차는 별도 `docs/RELEASING.md` 작업에서 확정한다. R 단계에 임의로 v0.6/v1.0을 배정하지 않는다.
- 릴리스 준비 시 기준 commit, 포함 PR, 지원 Aseprite/Go, 실행한 테스트와 미검증 범위를 기록하고 Unreleased 중 실제 포함된 변경만 해당 버전으로 옮긴다.
- 테스트·리뷰가 끝난 작업만 develop에 통합한다. 배포 검증 후 사용자 병합 지시/승인에 따라 main에 반영하고, 태그와 게시 artifact를 확인한 뒤 배포 완료를 기록한다.

## 관리 체크리스트

- [x] 50개 MCP 도구와 v0.1.0–v0.5.0 태그 코드 대조
- [x] 공식 기능군과 부분/미지원 범위 문서화
- [x] 구현 상태와 태그 포함 여부 분리
- [x] NEXT_STEPS의 중복 실행 순서를 이 로드맵에 통합
- [ ] OPS-01: CI 버전 artifact (로그 출력은 구현됨)
- [ ] upstream #19: 제출 범위 결정
- [x] SAFE-01: warnings 구현 (PR #11 develop 병합 완료, [검증 범위](WARNINGS.md))
- [x] GAP-10: capability preflight 구현 (PR #13 병합 완료; 하한 실행 추가 검증 완료, NEXT_STEPS 참조)
- [x] SAFE-05: 파일 변경 보호 구현 (PR #14 develop 병합 완료; [검증 범위](FILE_PROTECTION.md))
- [x] BUG-04/05 수정·회귀 검증 및 PR #16 병합
- [ ] BUG-01 → BUG-03 → BUG-02 수정 및 실제 Aseprite 회귀 검증
- [ ] SAFE-02–04: dry-run·snapshot·history/undo
- [ ] GAP-01–06: 편집·조회·export 확장
- [ ] OPS-02 및 R5 후보별 범위·검증 계획 확정
- [ ] 릴리스 절차 문서와 0.x 호환성 정책 확정

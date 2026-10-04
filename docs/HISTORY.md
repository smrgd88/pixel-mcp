# 작업 이력 및 undo (SAFE-04)

상태: PR #24로 develop `7f439a0`에 병합 / Unreleased. SAFE-03 snapshot/restore는 PR #23으로 develop `e0304bd`에 병합됐다.

## 사용법과 범위

설정 파일에 `"enable_history": true`를 추가하면 기존 sprite를 편집하는 MCP 호출의 성공한 파일 변경을 자동 기록한다. 기본값은 false다. 자동 백업의 디스크·시간 비용과 용량 초과 시 편집 거부를 기존 사용자에게 묵시적으로 추가하지 않기 위한 선택이다. 저장소 경로는 SAFE-03의 optional `snapshot_dir`를 공유한다.

1. 편집 도구를 호출한다. 기존 응답과 warnings 형식은 유지한다.
2. `list_operation_history`에 `sprite_path`를 전달한다.
3. 목록의 최신 `state=applied` 항목의 `operation_id`를 `undo_last_operation`의 `expected_operation_id`에 전달한다. `sprite_path`도 필수다.
4. 성공하면 `success`, 되돌린 `operation` (`state=undone`), `backup_snapshot`을 반환한다.

두 도구는 `enable_history=false`에서도 등록된다. 기록을 끈 뒤에도 기존 이력을 확인하거나 undo할 수 있다. list 응답은 `operations` 배열과 `recording_enabled`다. SAFE-04 도입 당시 도구 수는 54개에서 56개로 늘어났다. 현재 inventory는 [CAPABILITIES](CAPABILITIES.md)를 따른다.

대상은 기존 파일의 단일 sprite 편집 경로다: 그림·cel 위치/opacity(`set_cel_properties`)·레이어/프레임·선택/clipboard·팔레트·변형·감색·자동 shading·적용형 antialiasing·기존 sprite로의 image import. selection과 clipboard도 파일에 저장되므로 포함한다. 실패한 handler, 실제 파일 bytes가 같은 no-op, dry-run은 기록하지 않는다. get/analyze, export, save_as, downsample 출력, 새 canvas 생성, 수동 snapshot 생성/복원·삭제는 자동 편집 이력 대상이 아니다. raw Aseprite/Go API나 GUI의 미저장 작업도 대상이 아니다.

## 데이터와 보존

각 편집 전 snapshot의 metadata에 optional `operation`을 추가한다. 별도의 history DB와 snapshot 사이에 dangling ID를 만들지 않는다. 기존 snapshot metadata에는 필드가 없어도 정상이다.

- `operation_id`: canonical UUID v4. snapshot ID와 별개다.
- `tool`: 서버가 등록한 도구 식별자.
- `sequence`: 저장소 잠금 아래 현재 남은 기록의 최대값보다 큰 순번. clock 역행과 관계없이 목록은 순번 내림차순이다. 모든 과거 기록이 삭제되면 순번은 재사용될 수 있으므로 ID가 영구 식별자다.
- `created_at`: 편집 결과와 복원용 snapshot을 준비한 UTC 시각 (실행 시작 시각 아님).
- `state`: 정상 목록에는 `applied` 또는 `undone`.
- `before_sha256`, `after_sha256`: 정확한 파일 바이트 해시. 픽셀/의미 동등성 검사가 아니다.
- 목록 항목에는 `snapshot_id`, `expires_at`도 포함한다.

이력 항목에 원본 경로, 원시 도구 arguments, Lua, label, 이미지 데이터는 기록하지 않는다. 기존 snapshot metadata는 복원에 필요한 canonical 원본 경로를 여전히 보관하므로 암호화·익명화 저장소가 아니다. 기존 stderr logging 정책 전체 변경은 이번 범위에 없다.

SAFE-03의 100개 / 512 MiB / 7일 TTL을 수동 snapshot·자동 복원본·undo 전 백업이 함께 사용한다. 삭제/만료된 snapshot의 이력도 함께 사라진다. 최신 ID가 사라졌으면 undo는 `history_operation_mismatch`로 거부한다. 미만료 snapshot을 자동 eviction하지 않으며 undo를 위한 백업 공간이 없으면 원본을 보존하고 실패한다. 무기한 감사 로그나 무제한 undo stack이 아니다.

## 변경의 발행과 중단 복구

store·source·추가 입력 파일을 기존 전역 순서대로 잠근다. 같은 저장소의 자동 이력 편집은 서로 다른 sprite라도 직렬화된다. 용량 한도와 기록 순서를 보호하는 대신 동시 처리량이 줄 수 있으며 잠금 대기도 요청 timeout에 포함된다. handler는 SAFE-05의 private staging에서 실행한다. 성공한 handler가 bytes를 바꾼 경우에만 편집 전 snapshot과 `pending` 작업 기록을 fsync/원자적 metadata 교체로 저장한다. 그 다음 원본을 atomic replace하고 기록을 `applied`로 확정한다. 원본 발행 전 오류는 원본을 보존하고 이번 복원본을 정리한다. 새 기록의 준비가 실패하면 원본 발행도 금지한다.

원본 발행 후 metadata 확정이 실패하거나 취소되면 이미 성공한 편집을 실패로 보고하지 않는다. 저장된 `pending`을 남기며 같은 sprite의 다음 기록/list/undo가 현재 파일과 전후 해시를 비교한다.

| 처리 중 기록 | 현재 bytes | 조치 |
| --- | --- | --- |
| `pending` | after | `applied` 확정 |
| `pending` | before | 성공한 편집 이력에서 제외; snapshot은 일반 복원본으로 보관 |
| `undo_pending` | before | `undone` 확정 |
| `undo_pending` | after | `applied` 유지 |
| 어느 쪽이든 | 전후 어느 것과도 다름/읽을 수 없음 | `history_recovery_required`; 자동 파일 변경 금지 |

미확정 전이는 정상 성공 이력으로 노출하지 않는다. 위 판별은 실제 파일의 최종 상태를 기준으로 하며 네트워크 응답 수신 여부나 native undo stack 복구는 보장하지 않는다. 모호한 상태에서는 `list_snapshots`로 복원본을 확인하고, 사용자 의도에 따라 SAFE-03의 수동 restore 또는 해당 snapshot 삭제로 정리한다. 삭제하면 해당 작업 이력도 사라진다.

undo는 최신 미복원 작업 ID와 현재 파일이 그 작업의 after 해시인지 먼저 검사한다. 오래된 ID·외부 변경·snapshot 손상은 원본을 유지하고 거부한다. SAFE-03 restore의 복원 전 백업·원본 변경 감지·atomic replace를 재사용하고, 교체 직전 `undo_pending`을 저장한다. 확정 쓰기 실패는 위 방식으로 복구한다. 동일 ID 재시도로 다음 과거 작업을 되돌리지 않는다. 다음 작업을 되돌리려면 다시 목록을 읽고 그 ID를 전달한다.

한 저장소·파일을 사용하는 서버는 모두 SAFE-04 이상으로 업그레이드해야 한다. 이전 서버가 metadata를 수정하거나 기록 없이 파일을 편집하면 최신 기록과 불일치할 수 있다. 파일시스템 전원 장애·다중 파일 일괄 트랜잭션·redo·개별 operation 선택 rollback은 범위 밖이다. 강제 종료 시 snapshot 디렉터리의 임시 metadata 파일은 해당 snapshot 삭제/만료 때 함께 정리된다.

## 검증과 셀프리뷰 (2026-09-30)

2026-09-30 검증: Linux Go 1.25.14/Aseprite 1.3.18.3-dev에서 build·vet·전체 race/coverage·전체 integration 통과 (pkg/tools integration 289.287초). 최소 Aseprite 1.3.17.2의 history/snapshot MCP 회귀 통과 (5.464초). macOS arm64/Aseprite 1.3.18.2-arm64의 history/snapshot core 및 history/snapshot/dry-run MCP 회귀 통과. 최소 버전·macOS는 관련 회귀 범위다.

- 실제 flatten·감색·image import의 기록/복원, 기존 warnings와 dry-run/read/export 제외, 기록 비활성화·재시작 뒤 undo, 원본 bytes와 Aseprite 재열기를 확인했다. server 등록 도구는 56개다.
- core 회귀: 두 편집의 역순 undo, 동일 ID 재시도 차단, 외부 변경/삭제/손상/quota 거부, 성공 no-op·실패 제외, pending/undo_pending의 모든 endpoint 판별, 동시 편집 순번·체인, 취소/잠금 대기·만료를 검증했다.
- 리뷰에서 BE-P1-001을 발견했다: undo 최초 해시 확인과 restore staging 사이에 외부 편집이 들어오면 stageFile이 그 새 상태를 기준으로 받아들일 수 있었다. restore 내부에서 expected source hash를 다시 검사하도록 수정했다. `TestHistoryRestoreRechecksExpectedStateInsideStaging`이 이 경합 경계에서 외부 bytes와 snapshot 수 보존을 검증한다. 상태 VERIFIED.
- `convergent-code-review`: base/initial HEAD/merge-base `e0304bd7854bd42ab8b13d107c162180f6161f90`; 33개 변경 파일, pkg/aseprite·pkg/tools·pkg/config·pkg/server 및 문서. 최초 전체 1회, repair cycle 1회, repair-diff 1회, closure 1회, fresh 전체 1회 (review pass 총 4회). 최종 후보 뒤 의미 있는 변경/인증 무효화 0회.
- finding 흐름: 초기 P1 1건 → 수정/검증 후 P1 0건 → closure/fresh 신규 0건. P0, 재오픈, 수리로 생긴 결함, 미해결 finding은 0건. 확인된 차단 원인을 검증하고 전체 계약 대조·테스트가 완료돼 종료했다.
- Reviewed: 새 history 저장/도구·snapshot metadata/restore guard·config·server 등록·공통 wrapper와 각 도구의 config 전달, 관련 회귀/문서. Context: 기존 file_protection의 lock/staging/replace, snapshot 보존 정책, MCP fixture와 timing wrapper. Changed during repair: snapshots.go, history.go, history_test.go. Excluded: plugin 저장소, 기존 stderr 로그 정책 전체 개편, GUI/native undo, redo, 전원 장애 내구성, 다중 출력 rollback.
- 문서 상대 링크와 diff 공백 검사 통과. 과거 기록 전체가 만료/삭제되면 이력이 사라지고, 같은 저장소의 편집은 직렬화된다. 이러한 보존·동시성 한계는 위 계약을 따른다.

최종 판정: PASS.

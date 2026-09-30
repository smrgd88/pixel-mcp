# Snapshot / restore (SAFE-03)

상태: PR #23으로 develop `e0304bd`에 병합, Unreleased. SAFE-02 dry-run은 PR #21로 develop `1c410b6`에 병합했다.

## 목적과 범위

여러 MCP 호출 사이에 저장된 파일을 복원한다. Aseprite의 프로세스별 undo stack과 별개이며, 편집기의 미저장 변경은 포함하지 않는다. Go에서 파일 전체를 복사하므로 레이어·프레임·팔레트·metadata를 재인코딩하지 않는다. 저장 파일의 바이트 보관이며 Aseprite 형식 검증/복구나 임의 파일 import 도구는 아니다. 호출자는 sprite 파일을 지정해야 한다.

## MCP 계약

| 도구 | 입력 | 성공 응답 |
| --- | --- | --- |
| `create_snapshot` | `sprite_path`, optional `label` (UTF-8 256 bytes 이하) | `success`, `snapshot` |
| `list_snapshots` | optional `sprite_path` | `snapshots` 배열 (없으면 `[]`), 오래된 순 / 같은 시각은 ID 순 |
| `restore_snapshot` | `sprite_path`, `snapshot_id` | `success`, `snapshot_id`, `backup_snapshot` |
| `delete_snapshot` | `snapshot_id` | `success`, `snapshot_id` |

snapshot metadata: `snapshot_id` (소문자 canonical UUID v4), `sprite_path` (canonical 원본 경로), `created_at`, `expires_at` (UTC), `size_bytes`, `sha256`, optional `label`.

복원은 **존재하는 원래 canonical 경로에만** 허용한다. 다른 파일/새 경로로의 복원과 삭제된 원본의 재생성은 이번 범위에 없다. symlink 입력은 canonical 원본으로 해석한다. create는 읽기 전용 파일도 가능하지만 restore는 쓰기 가능·단일 hard link 조건을 만족해야 한다. 복원 후 현재 원본의 permission bits를 유지하며 이전 mtime/owner/ACL/xattr 복원은 약속하지 않는다. 같은 사용자 권한으로 접근 가능한 파일이 대상이며 별도 tenant 경계는 없다.

## 복원과 실패 보호

1. 저장소와 원본의 canonical 경로 잠금을 정렬된 순서로 함께 획득한다. 서버/프로세스가 달라도 같은 잠금을 사용한다.
2. UUID·metadata·소속 원본·만료·저장 파일 형태를 검증한다.
3. 원본과 같은 디렉터리의 private staging에 snapshot을 복사하면서 SHA-256/길이를 검증한다.
4. 현재 원본의 `before restore <ID>` 백업을 snapshot 저장소에 먼저 발행한다. 한도/권한/쓰기 오류로 백업이 실패하면 원본은 바뀌지 않는다.
5. 기존 SAFE-05의 원본 변경 감지·fsync·atomic replacement로 교체한다. 외부 편집이 감지되면 실패한다.

저장소와 원본이 다른 볼륨에 있어도 바이트를 원본 옆 staging으로 복사한 뒤 같은 볼륨에서 rename한다. 실패한 atomic replacement를 원본 truncate/copy로 대체하지 않는다. 복원 실패 직전에 백업이 발행됐으면 남겨 두며 `list_snapshots`로 찾을 수 있다. 성공 응답을 받지 못했을 때 맹목적으로 재시도하면 백업이 하나 더 생길 수 있다. idempotency token은 이번 범위에 없다.

create는 private `.pending-<UUID>` 디렉터리의 데이터/metadata를 쓰고 sync한 뒤 UUID 디렉터리로 rename한다. 정상 오류는 pending을 정리한다. 다음 create/list/restore는 잠금 안에서 이전 프로세스의 pending 디렉터리를 정리한다. 프로세스 중단 전후에 source의 일부만 쓰인 상태를 노출하지 않지만, 전원 장애까지 견디는 파일시스템 트랜잭션이나 저장소·원본 두 곳의 일괄 commit은 보장하지 않는다.

## 보존 정책과 설정

- 기본 저장소: `os.UserConfigDir()/pixel-mcp/snapshots`. 일반 재시작을 넘어 유지되며 `temp_dir`와 분리한다.
- optional config `snapshot_dir`: 별도 절대 경로. 같은 저장소를 공유할 서버는 같은 경로를 사용한다. 권한 문제가 생기면 snapshot 호출이 실패하며 다른 MCP 도구 등록을 막지 않는다.
- 상한: 저장소 전체 **100개 / 원본 바이트 합계 512 MiB**, 개별 snapshot TTL **7일**. 이번 MCP 설정에서는 고정 정책이며 per-call override는 없다.
- create/list/restore에서 만료된 snapshot을 lazy cleanup한다. 백그라운드 타이머는 없으므로 호출이 없으면 만료 파일이 디스크에 남을 수 있다.
- 미만료 snapshot 자동 eviction은 없다. restore의 사전 백업도 동일한 한도를 사용한다. 가득 차면 명시적으로 불필요한 snapshot을 삭제해야 한다.
- 저장소·snapshot 디렉터리는 Unix 0700, 데이터/metadata는 0600으로 생성한다. 이미 있는 저장 경로가 공개 권한이면 자동 chmod하지 않고 거부한다. snapshot 디렉터리 및 파일의 symlink, 저장 파일의 hard link, 비정규 파일을 거부한다. ID는 경로로 직접 해석하지 않는다.
- metadata에 원본 절대 경로가 포함된다. 같은 계정의 악의적인 저장소 변조를 막는 암호화/인증 저장소는 아니다. checksum은 데이터 무결성 검사이며 서명이 아니다.
- 손상/예상 밖 저장 항목은 조용히 덮어쓰거나 버리지 않고 오류로 반환한다. UUID 디렉터리가 남았다면 명시적인 delete로 손상 snapshot을 제거할 수 있다. 미지 파일은 운영자가 확인한다.

기존 편집 응답과 warnings 계약은 변경하지 않는다. 신규 도구의 오류는 기존 MCP 오류 경로를 사용한다 (`snapshot_invalid`, `snapshot_capacity`, `snapshot_source_mismatch`, `file_changed` 등의 메시지; 만료/삭제/없는 ID는 조회 실패). SAFE-04 구현 브랜치에서는 metadata에 optional `operation`을 연결한다. 자동 복원본도 같은 보존 한도를 사용하며 [HISTORY](HISTORY.md)를 따른다.

## 검증과 셀프리뷰 (2026-09-28)

- 기준: develop `1c410b623fa413dd8e88a2eb7b255329415a5941`; SAFE-03 구현과 관련 문서·config·등록·테스트 17개 파일.
- Linux Go 1.25.14 / Aseprite 1.3.18.3-dev: build, vet, 전체 unit/race/coverage, 전체 integration 통과 (pkg/tools 279.354초). 최종 테스트 보정 후 관련 패키지 race와 snapshot MCP 회귀 재검증 통과.
- 최소 Aseprite 1.3.17.2 snapshot MCP 회귀 통과 (1.056초). macOS arm64 / Aseprite 1.3.18.2-arm64 저장소·MCP 회귀 통과. 이 두 환경은 관련 회귀 범위다.
- 실제 native 파일의 레이어·프레임·metadata를 바이트 단위로 복원하고 Aseprite 재열기를 확인했다. 별도 MCP 서버에서 snapshot 재발견, 복원 전 백업으로 되돌리기, indexed 입력, 54개 도구 등록·schema를 검증했다.
- quota 동시 요청, 만료·orphan 정리, 체크섬·metadata 손상, 경로/권한/hard-link/symlink 거부, 취소, commit 실패 시 원본·백업 보존, Linux 다른 파일시스템 복원 통과. 문서 상대 링크·diff 공백 검사 통과.

`convergent-code-review`의 backend lens로 최초 검토, 수정 diff 검토, closure 및 최종 전체 검토를 수행했다. 최초 전체 검토 1회, 수정 cycle 2회, 수정 diff 검토 2회, closure 1회, fresh 전체 검토 1회 (총 review pass 5회). 최종 후보 선정 뒤 의미 있는 변경/인증 무효화 0회.

| ID | 판정 | 원인과 확인 |
| --- | --- | --- |
| BE-P2-001 | VERIFIED | 새 snapshot의 초기/최종 해시가 크기 제한·취소를 확인하지 않음. bounded/context-aware reader 적용; 제한 및 취소 회귀 통과 |
| TEST-P2-002 | VERIFIED | macOS `/var` alias를 canonical 경로와 직접 비교해 native 검증 실패. canonical 기대값으로 보정 후 macOS 회귀 통과 |

최초 P2 1건, 검증 중 추가 P2 1건. P0/P1·재오픈·수정으로 생긴 제품 결함·최종 추가 finding·미해결 finding 0건. 검증을 막던 테스트 기대값 보정을 두 번째 수정 cycle로 처리했다. 확인된 finding을 모두 검증하고 최종 계약 대조를 완료해 종료했다.

Reviewed: snapshot 저장소·MCP 도구·config·server 등록 및 관련 테스트/문서. Context: 기존 file_protection의 잠금·staging·atomic replace·regular-file 검사, 기존 MCP fixture·timing wrapper. Changed during repair: snapshots.go와 snapshot 회귀 테스트 및 config 검증 테스트. Excluded: SAFE-04 history/undo, 미저장 GUI 변경, 플러그인 저장소, 기존 파일 보호 구조 전체 리팩터링, 전원 장애 내구성.

최종 판정: PASS.

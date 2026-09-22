# 파일 변경 보호 (SAFE-05)

PR #14로 develop `7e60ad9`에 병합했다. 릴리스 버전은 아직 Unreleased다.

## 보호 단위

MCP의 동일 파일 읽기/쓰기·쓰기/쓰기는 **호출 전체**를 배타 잠금으로 직렬화한다. 감색/자동 shading처럼 여러 Lua 프로세스와 Go 처리가 있는 호출도 하나의 작업 복사본을 사용한다. 작업 callback이 오류 없이 끝나고 요청이 취소되지 않은 경우에만 원본을 교체한다. 응답 파싱 오류, Lua 저장 후 오류, 프로세스 실패, timeout/cancel 시에는 원본을 교체하지 않는다.

| 경로 | 적용 |
| --- | --- |
| 기존 sprite 수정 도구 | 원본과 같은 파일시스템의 임시 디렉터리에서 작업, 성공 시 파일 하나를 atomic replace |
| 조회·분석·export의 source | 같은 배타 잠금으로 writer와 직렬화. 읽기만 할 때는 원본 복사·교체 안 함 |
| `save_as`, 명시적 output_path의 `downsample_image`, `export_sprite` | 단일 출력 파일도 staging 후 교체. 응답에는 요청한 출력 경로를 반환 |
| `export_spritesheet` | source·texture·파생 JSON 경로 잠금 및 alias 충돌 거부. texture+JSON 다중 파일 묶음의 atomic publish는 미포함 (R4 export 정책에서 처리) |
| `create_canvas`, output_path가 없는 downsample | 서버가 생성하는 고유 새 파일. 이번 기존 파일 보호의 rollback 대상 아님 |
| 직접 `Client.ExecuteLua(..., spritePath)` | 외부 operation context가 없으면 Lua 호출 하나를 보호. context가 있으면 같은 작업 복사본 재사용 |

경고·MCP 입출력 schema와 도구 수(50개)는 유지한다. 공통 wrapper는 기존 input struct의 SpritePath/SourcePath/ReferencePath/OutputPath/ImagePath string 필드를 사용하며, 임의 JSON 문자열에서 경로를 추출하지 않는다. 새로운 도구는 같은 필드 계약과 wrapper를 사용하고, read-only 도구는 명시적 목록에 등록해야 한다.

직접 Lua에 별도 저장 경로를 하드코딩하거나 raw ExecuteCommand를 사용하는 외부 라이브러리 호출은 임의 파일 전체의 sandbox가 아니다. 보호 대상은 예약한 source/단일 output이다. export 이미지 시퀀스·sheet 부속 JSON 등 여러 출력의 일괄 rollback은 이 단계의 범위 밖이다.

spritesheet의 JSON 파일은 현재 generator가 include_json=false일 때도 경로를 전달하므로 항상 잠금·원본 alias 검사 대상이다. source/texture/JSON이 서로 같은 파일을 가리키면 export 전에 거부한다. 이 검사는 다중 파일 atomic publish를 의미하지 않는다.

## 잠금과 경로

- absolute path와 symlink를 정규화한다. 새 출력은 가장 가까운 존재하는 부모부터 정규화한다. symlink 뒤의 `..`는 symlink 해석 후 처리한다. dangling symlink와 Windows drive-relative 경로(`C:foo`)는 거부한다.
- 여러 경로는 정렬된 순서로 잠근다. source/output/image가 같은 경로이면 한 번만 잠근다. 중첩 호출은 처음 예약한 경로 안에서만 가능하다.
- Linux/macOS는 flock, Windows는 LockFileEx를 사용한다. 같은 사용자·호스트·cache 디렉터리의 협력하는 서버 프로세스 간에도 유효하다. 프로세스가 죽으면 OS가 잠금을 해제한다.
- 잠금 파일은 사용자 cache의 `pixel-mcp/file-locks`에 둔다. 파일 이름은 canonical path의 SHA-256이다. inode 교체 중 잠금이 갈라지지 않도록 실행 중에 잠금 파일을 삭제하지 않는다. metadata 파일이 남아 있어도 잠금이 유지된다는 뜻은 아니다.
- macOS/Windows의 잠금 키는 소문자로 정규화한다. case-sensitive 볼륨의 서로 다른 대소문자 파일도 직렬화될 수 있지만 잘못된 파일을 교체하지 않는다.
- snapshot/복사 전에 일반 파일인지 검사하며 FIFO/device는 열기 전에 거부한다. Unix에서는 nonblocking open과 열린 handle의 재검사도 적용한다.
- hard link가 있는 파일은 atomic replace가 alias를 분리하므로 쓰기를 거부한다. 읽기 전용 파일은 쓰기를 거부하며, 조회는 가능하다.
- 각 MCP 호출의 설정 timeout에는 lock 대기·probe·Lua 및 후처리가 포함된다. 대기는 context 취소/timeout으로 중단할 수 있고 이미 취득한 잠금은 해제된다.

## 저장과 실패 경계

1. 원본 파일 상태와 SHA-256을 기록하고 같은 부모에 `.pixel-mcp-stage-*` 디렉터리를 만든다. 기존 sprite 수정은 원본을 복사하지만, 새 출력 생성은 staging 파일을 미리 만들지 않고 callback이 반드시 새 파일을 생성해야 한다. 기존 출력 파일을 새 결과로 간주하지 않는다.
2. 수정 작업은 복사본을 열어 저장하고, 출력 작업은 새 staging 파일을 생성한다. 원본 이름/확장자를 유지하며 staging 디렉터리는 private이다.
3. 성공 후 staged 파일이 regular/non-empty인지 확인한다. 원본 경로·파일 identity·내용·mode가 달라지면 `file_changed`로 교체를 거부한다.
4. 기존 파일의 permission bits를 보존하고 staged 파일을 fsync한 뒤 rename/MoveFileEx로 교체한다. 실패 시 원본을 먼저 삭제하거나 덮어쓰기 copy로 fallback하지 않는다.
5. 정상 완료·오류·취소·panic unwind 시 임시 디렉터리를 정리한다. atomic replacement 완료가 commit point다. 최종 취소 검사 후 교체에 진입한 작업은 새 취소 요청과 경쟁할 수 있으며, 이미 완료된 저장은 되돌리지 않는다.

서버 강제 종료 시 원본은 commit 전 상태 또는 commit 후 상태로 남으며, 고아 staging 디렉터리는 남을 수 있다. 살아 있는 작업의 디렉터리를 자동 삭제하지 않는다. 원본이 정상인지 확인하고 서버 작업이 없는 상태에서 정리한다. 고아/TTL 자동 정리는 후속 보존 정책 범위다.

보장 범위는 로컬 파일시스템의 단일 파일 교체와 협력하는 pixel-mcp 호출이다. Aseprite GUI나 다른 프로그램은 이 잠금을 따르지 않는다. 외부 변경 검사는 덮어쓰기 위험을 줄이지만 마지막 검사와 rename 사이의 비협력 writer 경쟁까지 배제하지 않는다. 전원 장애 durability, 네트워크 파일시스템/다른 호스트 잠금, ACL·확장 속성 전체 보존은 이번 보장이 아니다.

## 검증

- atomic replace 권한 실패 주입 후 원본 보존·잠금 재획득, commit/오류/cancel/panic, 빈/삭제/symlink staging, 외부 원본 변경, mode 보존과 임시 파일 정리.
- canonical symlink alias 동시 수정, 다른 파일의 독립 진행, 잠금 timeout 후 재획득, hard-link 쓰기 거부, read-only 조회.
- 별도 Go 프로세스가 작업 복사본을 쓴 뒤 강제 종료되어도 원본 불변 및 잠금 재획득.
- 실제 Aseprite가 저장한 뒤 Lua 오류/timeout, 여러 Lua 단계 뒤 handler 오류 시 원본 불변.
- 실제 MCP concurrent add_layer의 변경 유실 방지와 save_as의 새/동일 target 및 반환 경로.

Linux amd64에서 실제 실행 검증한다. darwin amd64/arm64, linux arm64, windows amd64는 cross-build와 각 플랫폼 실행 검증을 구분해 보고한다.

2026-09-22 실행 기록: Linux amd64 / Go 1.25.14 / Aseprite 1.3.18.3-dev에서 전체 build·vet·unit/race/coverage·integration 통과 (pkg/tools integration 157.153초). 최종 경로/교체 실패 회귀를 포함한 집중 race·통합 테스트도 통과했다. 최종 코드의 linux amd64/arm64, darwin amd64/arm64, windows amd64 build를 확인했으며 다른 OS의 실행 검증은 하지 않았다.

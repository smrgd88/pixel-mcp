# 출력 세트 crash 복구 설계 (SPIKE)

작업: `[BE][SPIKE] 출력 세트 crash 복구 설계` · 2026-10-18.
기준: develop `fcb83861df08b4ee897f162d9a175bd9d149ed96` (PR #37 포함).

**설계·현행 강제 종료 실험·복구 판단 모델의 검토 결과다. Production 복구 기능은 구현하지 않았다.** 서버에는 새 journal, 자동 재시작 복구, 복구 도구, crash-safe cleanup이 추가되지 않는다. 아래 상태/코드/설정 이름은 후속 구현 제안이며 현재 MCP 호출에 사용할 수 없다.

## 문제와 실제 관찰

`pkg/aseprite/output_files.go`의 `WithOutputFiles`는 모든 출력을 staging/검증한 뒤 순차 교체한다. 기존 파일은 각 출력 부모의 private `.pixel-mcp-stage-*` 안에 `.original-backup`으로 복사·동기화한다. 일반 오류는 역순 rollback하며, 실패하면 [현재 복구 참조](ERRORS.md)를 반환한다.

그러나 `committed`는 메모리 상태다. rename 직후 `committed=true`를 설정하기 전에도 프로세스가 종료될 수 있다. SIGKILL은 defer와 일반 오류 rollback을 실행하지 않는다. 현재 구현에는 출력 세트를 재구성할 영속 registry/journal이 없고 directory-entry durability를 위한 전체 directory sync 순서도 없다.

새 `output_crash_spike_test.go`는 테스트 자식 프로세스에서 실제 `withOutputFilesReplace`와 `replaceFile`을 호출한다. 부모는 stdout checkpoint를 확인한 뒤 해당 자식만 SIGKILL한다. fixture는 두 일반 파일이며 image encoder가 아닌 **출력 발행 primitive**를 검증한다. 새 파일 세트/기존 파일 세트를 나눠 8개 조합을 실행한다.

| kill 경계 | 남은 출력 | staging/백업 | 관찰 |
| --- | --- | --- | --- |
| callback이 두 staged 파일을 작성한 뒤 | 기존 세트 또는 둘 다 없음 | stage 2개, 기존이면 backup 2개 | defer 정리 없음 |
| 첫 rename 직전 (전체 staged 검증 후) | 기존 세트 또는 둘 다 없음 | 위와 같음 | 아직 발행 안 됨 |
| 첫 rename 직후, callback 반환 전 | 첫 출력만 새 값, 두 번째는 옛 값/없음 | 위와 같음 | 세트 혼합, 메모리 ack 전 crash 재현 |
| 두 번째 rename 직후, callback 반환 전 | 두 출력 모두 새 값 | 위와 같음 | 응답·commit 기록 없이 새 세트 가능 |

모든 조합에서 자식 종료 후 기존 `WithFileLocks`를 부모가 재획득했다. 잠금이 풀리는 것과 세트 복구는 별개다. 테스트는 원래 바이트와 백업 존재를 assert하며 생성 경로는 `t.TempDir()` 안에만 둔다. staged 생성 도중, backup 생성 도중, cleanup 도중의 실제 kill 및 전원 장애는 이번 재현 범위 밖이다.

## 채택할 목표와 제외 범위

1. 협력하는 같은 사용자/호스트 서버의 다음 접근 전에 미완료 출력 transaction을 발견하고 복구/격리한다.
2. 영속 `COMMITTED`가 없으면 이전 세트로 rollback, 있으면 새 세트를 유지한다. 모든 rename이 끝났어도 commit marker가 없으면 rollback할 수 있다.
3. 복구 자체가 다시 종료돼도 재진입할 수 있어야 한다. 복구 불확실성·외부 변경·검증 실패는 fail-closed로 보존한다.
4. 고정된 출력 경로의 다중 rename을 OS 차원에서 하나로 atomic하게 만들지는 않는다. 외부 reader는 작업/복구 도중 혼합 세트를 볼 수 있다.
5. GUI 등 비협력 writer의 최종 검사–rename 경쟁과 ABA는 [별도 검토](NEXT_STEPS.md#태그-후속-todo) 대상이다. 이번 설계의 lock/hash가 이를 해결한다고 주장하지 않는다.
6. 첫 구현은 검증된 로컬 파일시스템의 **프로세스 종료 복구**부터 제공한다. 전원 장애·볼륨 분리·네트워크 파일시스템·다른 호스트·전체 ACL/xattr 복원은 별도 수용 기준 없이는 보장하지 않는다.

일관된 세트의 **관찰 원자성**이 필수라면 고정 texture/JSON 경로 모델을 유지할 수 없다. 별도 immutable generation 폴더를 모두 생성한 뒤 작은 manifest 하나를 교체하고, 소비자가 manifest의 generation을 고정해 모든 파일을 읽는 별도 opt-in API가 필요하다. 기존 게임 엔진/스킬의 경로 소비 계약이 바뀌므로 이번 rollback journal 설계와 섞지 않는다.

## 영속 기록과 발견 경로 제안

- 사용자별 private transaction registry를 두고 transaction UUID만 공개한다. snapshot/history 저장소와 분리한다. registry의 최종 위치·설정 키는 구현 PR에서 확정하며 기존 설정을 암묵적으로 이동하지 않는다.
- journal은 schema version, operation ID, generation/상태, 출력 순번, 요청/정규화 대상 경로, 부모 디렉터리 identity, stage 디렉터리/후보명, 기존 파일 존재·identity·mode·size/hash, 새 staged identity·mode·size/hash, backup identity/hash를 기록한다. 파일명은 JSON 문자열로 직렬화하며 문자열 prefix로 디렉터리 포함 여부를 판정하지 않는다.
- registry에는 비공개 절대 경로가 필요하지만 응답·일반 로그에는 경로를 노출하지 않는다. 현재 `output_index`와 recovery reference 방식은 유지하고 새 operation ID와 상태를 additive하게 설계한다.
- schema를 읽는다고 경로를 신뢰하지 않는다. 파일/디렉터리 owner·권한·regular type, no-symlink/no-hardlink, 허용 schema/길이/출력 수, 중복/alias/source 충돌, canonical 부모/identity, stage가 실제 해당 target 부모의 **기록된 private 디렉터리**인지 검사한다. 동일 UID의 악성 프로세스에 대한 완전한 보안 경계는 아니다.
- PID·mtime·UUID 모양만으로 죽은 작업을 판단하지 않는다. 실행/복구는 transaction lock과 기존 정렬된 **전체 출력 잠금**을 취득한다. 살아 있는 transaction이면 기다리거나 timeout한다. lock 파일을 삭제해 잠금을 초기화하지 않는다.
- 출력 키→transaction 발견 인덱스가 필요하다. 첫 rename 전 모든 출력 키의 참조를 영속화하고 모든 협력 reader/writer의 접근 경로에서 검사해야 한다. snapshot registry 전체를 매번 무제한 스캔하거나 프로세스 시작 시 전체 사용자 디스크를 탐색하지 않는다. 인덱스 불일치/유실은 복구 오류로 격리한다.
- registry·출력 부모가 서로 다른 볼륨이어도 rename은 각 출력의 같은 볼륨 stage에서 수행한다. 중앙 commit 기록과 모든 출력 directory-entry의 durable ordering을 설계·검증해야 한다. 볼륨 일부가 없으면 세트를 일부만 성공 처리하지 않는다.

잠금 순서는 발견 인덱스 갱신용 짧은 registry lock → 이를 해제한 뒤 transaction ID 순 lock → canonical 전체 출력 순 lock으로 정한다. registry lock을 쥔 채 output/transaction lock을 기다리지 않는다. 출력 잠금을 이미 가진 경로에서 다른 transaction이 발견되면 기존 잠금을 해제하고 발견 generation을 다시 확인한 뒤 필요한 **합집합 전체**를 이 순서로 재취득한다. 일부 output lock을 쥔 채 추가 lock을 취득하지 않는다. 최종 잠금 아래 인덱스를 재검사해 새 overlap이 있으면 제한된 재계획 또는 timeout한다. 첫 발견 참조를 발행하는 writer도 같은 규칙과 전체 출력 예약을 사용해야 한다.

## 상태 전이와 기록 순서

| 상태 | 다음 동작 | 재시작 판단 |
| --- | --- | --- |
| `ALLOCATED` | 정규화된 대상·고유 stage 경로 계획을 먼저 기록하고 배타 생성, 백업/새 출력 작성 | 첫 publish 금지 상태. 대상 불변 확인, 기록된 임시물만 정리 가능 |
| `PREPARED` | 모든 backup·새 output·permissions·hash 및 format/JSON 검증 완료 | live는 옛 값이어야 함. 불일치면 충돌 |
| `PUBLISHING` | **첫 rename 전** 이 상태와 전체 발견 인덱스를 durable 기록 | 아래 관찰 기반 rollback |
| `COMMITTED` | 모든 rename과 부모 sync 후 commit marker 기록, 성공 응답 허용 | 새 세트 검증 후 유지. 응답 유실이 재실행 허가가 아님 |
| `ROLLING_BACK` | 관찰·backup을 검사하며 역순 복원/새 출력 제거 | 완료 hint보다 현재 identity/hash로 재진입 |
| `ROLLED_BACK` | 옛 상태 전체 확인 후 terminal marker 기록 | 옛 세트 유지, 안전한 임시물만 정리 |
| `CONFLICT` / `RECOVERY_FAILED` | 증거 보존·관련 출력 차단·수동 해결 안내 | 자동 덮어쓰기·TTL 삭제 금지 |

필수 순서:

1. 전체 경로·transaction lock 취득, 이전 미완료 세트 처리, 원본 상태 기록. `ALLOCATED`를 영속화한 뒤 기록된 stage 경로를 배타 생성한다. 생성 전 종료나 부분 생성은 대상 변경 없이 판별 가능해야 한다.
2. backup 복사→file sync, parent/stage directory sync, 렌더→전체 검증→file sync, `PREPARED` journal을 temp-write→sync→atomic replace→registry directory sync로 발행한다. 모든 오류는 publish 전에 중단한다.
3. `PUBLISHING`과 모든 발견 참조를 sync한다. 각 대상의 expected identity/hash/권한/alias를 확인하고 rename→출력 부모 directory sync를 수행한다. 각 output의 applied bitmap은 진단 hint이며 correctness의 근거가 아니다.
4. 새 세트 전체를 검사하고 `COMMITTED`를 durable 발행한다. 여기까지 성공한 뒤에만 성공 응답을 보낸다. commit marker 쓰기 실패는 commit 성공으로 추정하지 않으며 backup/기록을 유지한다.
5. terminal 상태에서만 cleanup한다. backups 제거보다 terminal marker가 먼저다. 완료 기록/tombstone은 cleanup 재시도와 응답 유실 구분에 필요한 보존 기간 동안 유지한다.

실제 fsync/directory flush는 OS별 adapter가 필요하다. 지원하지 않는 볼륨에서 조용히 무시하고 durability를 주장하지 않는다. `SIGKILL` 테스트 통과는 storage cache가 유지되는 프로세스 종료 근거이며 전원 장애 검증을 대신하지 않는다.

## Rollback·멱등성·충돌

`PUBLISHING`이지만 `COMMITTED`가 없으면 전체 세트를 preflight한 뒤 역순 복원한다. 해시만으로 신뢰하지 않고 경로·identity·mode·backup을 같이 검사한다.

| 관찰 | 제안 동작 |
| --- | --- |
| 기존 파일이 옛 identity/hash로 존재 | 이미 옛 값, 유지 |
| 기존 파일 대신 기록된 새 identity/hash가 존재, 유효한 backup 존재 | backup을 **다시 복사한** recovery stage를 sync 후 교체. 원본 backup은 terminal marker 전까지 보존 |
| 처음 없던 target에 기록된 새 identity/hash가 존재 | 이번 transaction의 파일만 제거, 부모 sync |
| 처음 없었고 지금도 없음 | 이미 복구됨 |
| byte는 같지만 다른 identity, mode/alias 변경, 예상치 않은 파일/삭제, backup 손상/누락 | 자동 복구 중단·증거 보존. same-byte ABA를 해시만으로 승인하지 않음 |
| commit marker 존재 + 새 상태 전체 일치 | 새 세트 유지, cleanup만 재개 |
| commit marker 존재 + 외부 변경 | 과거 backup을 자동 덮어쓰지 않음. 충돌 표시 후 수동 해결 |

복구 전 전체 충돌 검사 후 각 변경 직전에도 재검사한다. 실행 중 충돌이 생기면 앞서 복구한 항목을 다시 새 값으로 되돌리지 않고 `CONFLICT`와 남은 상태를 기록한다. 비협력 writer는 최종 check 이후 바뀔 수 있으므로 완전한 CAS 보장은 없다.

복원 직후 progress 기록 전에 다시 죽는 경우를 다루기 위해 recovery stage의 identity/hash와 복원 intent를 먼저 journal에 등록한다. 재시작은 그 identity를 옛 값으로 인정하고 다음 항목을 계속한다. backup을 rename으로 소모하면 이 재진입 증거가 사라지므로 복사본을 발행한다. 같은 bytes의 옛/새 출력은 기존 publisher처럼 no-op으로 명시 기록해 모호한 복구를 피한다.

테스트의 pure decision model은 publish가 시작될 수 있는 transaction의 before/new/absent hash 상태, commit 유무, 유효 backup과 복구 재진입을 검토한다. ALLOCATED/PREPARED의 상태 전이 정당성까지 검사하는 모델은 아니다. 이것은 판단 검토용이며 journal 파싱/identity/CAS/durability/실제 복원 로직을 구현하거나 증명한 것이 아니다. 실제 publisher kill 실험과도 별개다.

## 재시작·사용자 동의·정리 정책

- 첫 구현 제안은 자동 발견·관련 세트 차단과 **명시적 복구 적용**이다. 서버가 시작됐다는 이유만으로 사용자의 여러 output 디렉터리를 수정하지 않는다. 목록 조회는 side-effect 없는 진단, 복구 적용은 transaction ID·expected journal generation을 받는 별도 쓰기 동작으로 설계한다. 정확한 CLI/MCP 이름과 오류 코드는 후속 구현 PR에서 확정한다.
- 차단된 세트를 다른 export로 덮어써 증거를 지우지 않는다. 사용자는 진단을 보고 rollback을 선택하거나 수동으로 정리한 뒤 상태를 재검증한다. arbitrary path나 `force=true`로 외부 변경 검사를 끄는 복구 API는 첫 범위에서 제공하지 않는다.
- 취소·timeout·복구 실패로 backup을 삭제하지 않는다. 충돌 세트의 일부 파일 복구 완료를 세트 전체 완료로 보고하지 않는다. source sprite는 복구 대상이 아니며 output rollback을 source의 history undo라고 설명하지 않는다.
- `ALLOCATED`/`PREPARED`에서 대상 불변과 lock 미사용이 검증된 기록, 또는 검증된 terminal transaction만 cleanup 후보가 된다. cleanup 전후 generation/owner/path/identity를 다시 검사한다.
- 미해결·충돌·알 수 없는 `.pixel-mcp-stage-*`·손상 journal에는 자동 TTL 삭제를 적용하지 않는다. 이미 살아 있는 stage를 age/PID만 보고 제거하지 않는다. 기존 무journal stage는 자동으로 ownership/target을 추론해 복구하지 않고 수동 보존한다.
- registry·backup 용량 제한은 **첫 publish 전** 확인한다. 한도/공간 부족이면 새 작업을 거부하고 미해결 증거를 evict하지 않는다. 구체 한도·terminal 보존 기간과 migration은 구현 PR의 설정 계약으로 확정한다.

## 구현 후속과 완료 조건

| 후속 | 완료 조건 |
| --- | --- |
| 1. private registry·journal·발견 index | 경로/owner/schema 검증, 부분/손상 write, stage 생성 전후 kill, quota/볼륨 오류, 기존 journal 없는 버전 migration 테스트 |
| 2. publisher 연결 | PREPARED/PUBLISHING/각 rename 전후/각 directory sync/COMMITTED 전후/응답 전후 kill에 old/new/missing 상태의 기대값과 backup 보존 검사 |
| 3. recovery 연결 | 복구 copy/rename/remove/marker 전후의 실제 kill·재시작, 여러 번 적용, 3개 이상 출력·여러 부모·일부 missing·실패/취소, 출력별 exact bytes/permissions 확인 |
| 4. 외부 writer·reader·cleanup | before/after 사이 외부 overwrite/delete/symlink/rename/replaced-same-bytes, live lock·overlap·stale generation·unknown stage·terminal cleanup 재진입의 무손상 검증 |
| 5. API/진단·release | 목록/적용 계약·redaction·request ID·operation ID·MCP 호환 회귀 및 지원 OS/filesystem 증거, 사용자가 실제 새 바이너리를 실행해야 적용됨을 안내 |

단계 1–5를 별도 FEATURE로 구현한 뒤에만 runtime crash 복구 완료를 표시한다. 우선 로컬 Linux/macOS 프로세스 종료를 대상으로 하고 Windows·전원 장애·원격 볼륨 등은 실제 범위 결정 후 검증한다. 실험 테스트를 CI에 넣는 것만으로 자동 복구 기능이 생기는 것은 아니다.

## Plugin에 언제 무엇을 요청하는가

현재 별도로 요청한 plugin 작업은 **MCP PR #37 크기 예산 개선 pin/번들/스킬 동기화**다. 이 SPIKE의 완료는 그 작업의 새 runtime pin을 바꾸지 않는다.

SPIKE 후에는 선택적으로 기존 export 안내의 “강제 종료 시 자동 복구 없음·남은 backup을 지우거나 무조건 재시도하지 말 것”을 점검하는 문서 작업만 가능하다. 신규 복구 tool·schema·자동 재시도 예제를 지금 추가하지 않는다.

실제 복구 FEATURE가 검증·병합된 뒤에는 `[SHARED][FEATURE] journal 복구 계약 플러그인 동기화`를 요청한다: 해당 MCP commit pin/5개 bundle/provenance/schema, 진단→generation 확인→명시 적용→세트 재검증 workflow, 충돌 시 중단·비공개 경로 redaction·기존 backup 보존, 사용자가 승인하지 않은 자동 복구/재시도 금지 및 정상/충돌/반복 복구 실제 MCP 테스트. 개인 설치본 교체·릴리스는 추가 승인 단계다.

## 이번 검증 기록

- macOS arm64 / cached Go 1.25.0: `go test -count=1 -race ./pkg/aseprite -run 'TestOutputCrashSpike|TestOutputFiles' -v` 통과. 실제 publisher SIGKILL 8개 조합, pure recovery decision 7개 시나리오 및 충돌/backup 누락·반복 적용 모델, 기존 output rollback/취소/외부 writer/잠금/alias 회귀 포함. helper의 일반 직접 실행은 의도적 skip이며 kill 시나리오에서는 자식으로 실제 호출된다.
- Linux amd64 Docker (`pixel-mcp-ci:latest`, arm64 호스트에서 에뮬레이션) / Go 1.25.14: 같은 `go test -p 1 -count=1 -race ./pkg/aseprite -run 'TestOutputCrashSpike|TestOutputFiles' -v` 통과 (pkg/aseprite 3.191초). macOS 패키지 실행은 1.951초였다. Linux·macOS `go vet ./pkg/aseprite`도 통과했다. 임시 컨테이너에는 이 작업의 사본만 복사했고 사용자 config/Aseprite 실행 파일은 사용하지 않았다.
- 설계/테스트 자체 대조에서 다중 transaction/output 잠금 순서를 명시하고, pure model이 ALLOCATED/PREPARED 및 identity/durability를 검증하지 않는 한계를 보강했다. Production 파일 변경은 없다.
- 이 SPIKE는 Aseprite 편집/렌더 코드를 변경하지 않으며 native 이미지 export 통합·전체 unit/race/coverage·macOS 전체 suite·power-loss·실제 영속 journal 재생·Windows 실행을 수행/검증한 것으로 주장하지 않는다.

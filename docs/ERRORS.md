# 오류 코드와 요청 추적 (OPS-03)

상태: 구현 브랜치 / Unreleased. 기존 56개 도구의 성공 payload·warnings·output schema를 유지한다. 도구 추가는 없다.

## 성공 응답과 request ID

서버가 처리하는 각 `tools/call`에 UUID request ID를 만든다. 클라이언트가 보낸 `_meta`의 ID는 신뢰하거나 재사용하지 않는다. 응답 envelope의 `_meta["io.github.smrgd88.pixel-mcp/request_id"]`에 ID를 추가하며 기존 `content`·`structuredContent`는 수정하지 않는다. `enable_timing`과 무관하게 ID가 부여되고, timing 로그가 켜져 있으면 같은 ID를 사용한다.

request ID는 JSON-RPC의 `id`, history의 `operation_id`, snapshot ID와 별개다. 요청 하나에 대한 상관관계 식별자이며 재시도 idempotency key가 아니다.

## 도구 실행 오류

`isError=true`를 유지하고 `content`의 text를 아래 JSON 객체로 제공한다. 성공 output schema와 충돌하지 않도록 오류에는 `structuredContent`를 넣지 않는다.

```json
{
  "request_id": "UUID",
  "error": {
    "code": "history_conflict",
    "message": "The file has changed since the recorded operation."
  }
}
```

같은 `error` 객체를 `_meta["io.github.smrgd88.pixel-mcp/error"]`에도 넣는다. 기존 오류 text 문구는 안정적인 계약이 아니며, 새 클라이언트는 `code`로 분기한다. 오류의 원시 경로·입력값·Lua·stdout/stderr는 이 응답에 포함하지 않는다. 상세 원인 Go error는 내부에서 unwrap 가능하다.

| code | 의미 |
| --- | --- |
| `invalid_arguments` | SDK schema 또는 도구 입력 검증 실패 |
| `not_found`, `permission_denied` | 파일/리소스 없음, 접근 권한 부족 |
| `timeout`, `cancelled` | deadline 초과, 실행 취소 |
| `aseprite_execution_failed` | Aseprite 프로세스/명령 실행 실패 |
| `lua_error` | 이번 Lua의 구문/실행 오류 |
| `unsupported_aseprite`, `capability_probe_failed` | 지원 하한 미달, runtime 검사 실패 |
| `file_rollback_failed` | 일부 출력의 복구 실패. 자동 재시도 전에 수동 복구 확인 필요 |
| `file_changed`, `file_commit_failed`, `file_lock_scope` | 파일 변경 감지, 교체 실패, 잠금 범위 오류 |
| `snapshot_invalid`, `snapshot_capacity`, `snapshot_expired`, `snapshot_source_mismatch` | snapshot 검증·한도·만료·원본 불일치 |
| `history_invalid`, `history_scope`, `history_conflict`, `history_operation_mismatch`, `history_recovery_required` | history 검증·범위·파일 변경·순서·미확정 전이 |
| `operation_failed` | 아직 세부 분류가 없는 도구 오류/명시적 error result |
| `protocol_error` | 그 밖의 tools/call 프로토콜 오류 |

분류는 typed error/unwrap 및 context·filesystem sentinel을 사용한다. 임의 오류 문자열에서 `history_conflict:` 등을 검색해서 코드로 승격하지 않는다. typed rollback 실패를 최우선으로 반환한다. 복구 실패가 없을 때 timeout/cancelled를 우선하고, 그다음 명시적 domain code를 적용한다. snapshot 만료가 lazy cleanup으로 먼저 삭제된 경우에는 `not_found`일 수 있다. Lua 안에서 검사한 잘못된 레이어/프레임 등의 조건은 `lua_error`이며 모든 의미 오류를 `invalid_arguments`로 추측하지 않는다.

Lua 실행은 원본 script와 작은 loader를 private 임시 파일로 작성한다. loader의 `loadfile`/`xpcall`이 구문·실행 오류에 호출별 무작위 marker를 붙인다. Aseprite의 stdout 또는 stderr에서 이번 marker를 확인해 Lua 오류를 구분하며 두 임시 파일 모두 정상 오류/취소 종료 시 정리한다. 프로세스가 Lua 진입 전에 실패한 경우는 일반 실행 오류다. capability probe 실패는 기존 capability 분류를 유지한다.

## Rollback 실패의 복구 정보

`file_rollback_failed`는 취소·timeout·교체 실패보다 우선한다. 기존 출력 일부가 이미 교체됐고 원상복구도 실패한 상태이므로 일반 취소로 취급하거나 자동 재시도하지 않는다. `error.recovery`를 오류 text JSON과 namespaced error metadata에 동일하게 제공한다.

```json
{"code":"file_rollback_failed","message":"Some outputs could not be restored. Manual recovery is required; inspect the retained recovery directories before retrying.","recovery":[{"output_index":1,"directory":".pixel-mcp-stage-opaque","backup_file":".original-backup","rollback_failed":true}]}
```

- `output_index`: 내부 출력 목록의 1-based 순번. `export_sprite` 시퀀스는 프레임 출력 순서이며 단일 파일은 1이다.
- `directory`: 해당 출력의 **작업 시작 시 canonical 출력 부모 디렉터리** 아래에 있는 복구 폴더명. 절대 경로·원래 파일명은 공개하지 않는다.
- `backup_file`: 확인된 기존 원본 백업의 파일명. 신규 출력 또는 이미 사용/확인 불가인 백업은 필드를 생략하므로, 생략됐다고 임의로 백업 부재를 단정하지 않는다.
- `rollback_failed`: 해당 출력의 복구 실패 여부. 보존된 다른 출력의 폴더도 함께 나열하므로 false인 항목이 있을 수 있다.

백업을 자동으로 덮어쓰거나 삭제하지 않는다. 출력 경로의 symlink가 외부에서 바뀌었다면 현재 alias가 아니라 원래 canonical 출력 위치에서 폴더를 찾아야 한다. 복구는 수동 확인 대상이며 새 복구 API나 자동 복구를 추가하지 않는다. Go error/unwrap에는 기존 절대 경로 및 root cause가 보존되지만 기본 MCP 응답과 non-debug 로그에는 노출하지 않는다.

## JSON-RPC 오류와 경계

SDK의 필수 인자·타입/schema 검증 실패와 알 수 없는 도구는 기존 JSON-RPC 오류(`-32602`)를 유지한다. 다른 protocol 오류도 기존 wire code를 유지한다. `error.data`에 위의 `request_id`와 `error` 객체를 넣으며 원시 입력을 포함한 SDK message는 고정 공개 문구로 대체한다.

transport의 JSON 파싱 실패, tools/call 이전의 envelope 디코딩 실패, 다른 protocol method에는 이 middleware를 적용하지 않는다. 취소/연결 해제 때 응답 자체가 전달되지 않을 수 있으며 ID가 항상 클라이언트에 도달한다고 보장하지 않는다. 직접 Go API 호출 또는 Register*Tools만 별도 mcp.Server에 등록하는 호출자는 서버 middleware 계약을 자동으로 얻지 않는다. 제품 서버 `server.New`가 공통 경계다.

## 로그

- timing 여부와 무관하게 완료 로그에 같은 RequestID와 성공/오류 상태를 남긴다. 실패 로그는 공개 error code만 포함한다. 로그 레벨 설정은 그대로 존중한다.
- 번들 CLI의 non-debug 로그는 console/file sink 앞에서 속성을 정리한다. RequestID·SourceContext·Tool·Duration·Outcome·ErrorCode만 보존하고 나머지 값은 `[redacted]`, exception은 제거한다. 코드의 message template은 고정 문자열을 사용한다.
- `log_level=debug`는 상세 진단을 위한 명시적 선택이며 기존 입력·경로·process 출력이 포함될 수 있다. debug 로그 공유 시 주의가 필요하다. request ID와 공개 응답의 비노출 정책은 debug에서도 동일하다.
- stderr와 MCP stdout 분리는 유지한다. 외부 Go 사용자가 주입한 logger와 서버 시작 전 CLI 설정 오류까지 강제 필터링하는 변경은 아니다.

성공 응답 전체에 `success`/`details` 필드를 새로 강제하는 작업은 이번 범위에서 제외한다. 기존 클라이언트 호환성을 유지하기 위해 NEXT_STEPS에서 별도 검토 항목으로 관리한다.

## 검증과 리뷰 (2026-10-02)

2026-10-02 검증: Linux Go 1.25.14/Aseprite 1.3.18.3-dev의 build·vet·전체 race/coverage·전체 integration 통과 (pkg/tools 283.820초). 추가 request ID 주입 방어·shared result 보존·실제 timeout·CLI 파일 로그 회귀도 별도 통과. 최소 Aseprite 1.3.17.2와 macOS arm64/Aseprite 1.3.18.2-arm64에서 실제 Lua 오류 분류 및 서버 응답 관련 회귀 통과. 두 환경은 관련 회귀 범위다.

MCP SDK v1.4.1의 GetError·receiving middleware·JSON-RPC 변환 코드를 확인하고 실제 in-memory MCP session과 Aseprite로 검증했다. 테스트용 가짜 Aseprite 실행 파일은 사용하지 않았다. 오류 값에서 원시 입력/경로 제거, timing on/off 로그 ID 일치, 동시 요청 ID 분리, client ID 무시, schema/unknown tool의 protocol code 유지, 성공 structuredContent/warnings·기존 metadata 보존, Lua syntax/runtime·일반 process·timeout/cancel 분리와 임시 script/loader 정리를 확인했다. non-debug와 debug CLI 파일 로그의 차이도 확인했다.

셀프리뷰 (`convergent-code-review` backend lens): base/initial HEAD/merge-base `7f439a0df1ec0d2d24ef6dabba6de4d30c2484f9`; 40개 변경 파일. 초기 전체 검토 1회, 최종 fresh 전체 검토 1회 (총 2회). finding 기반 repair cycle·repair-diff·closure 0회 (해당 없음). 초기/추가/재오픈/미해결 finding 0건, P0/P1 0건. 최종 후보 확정 후 의미 있는 변경/인증 무효화 0회. 개발 중 발견한 Aseprite stdout 오류 출력 차이는 실제 회귀로 해결한 뒤 최종 후보를 검토했다.

Reviewed: diagnostics 코드/서버 경계·CLI 로그 필터·Aseprite command/Lua 구분·기존 오류의 typed code 부여·관련 테스트/문서. Context: pinned MCP SDK의 toolForErr/GetError/JSON-RPC 처리와 mtlog event/filter/sink, 기존 file protection/history/snapshot 및 timing 호출 경로. Changed during finding repair: 없음. Excluded: 플러그인 저장소, 성공 payload 전체 통일, stderr 시작 오류/외부 주입 logger의 일괄 정책, transport JSON 디코딩, 릴리스/모듈 경로 이전.

분류되지 않은 오류는 operation_failed로 반환하며 세부 오류 전체를 추측하지 않는다. debug 로그는 상세 값을 포함할 수 있고, 전달 자체가 취소된 응답에는 request ID 도달을 보장하지 않는다. 이 경계는 위 계약을 따른다. 검증과 최종 계약 대조가 완료되어 종료했다.

초기 후보 `9a3a4d3`의 PASS 기록이다. 이후 재리뷰에서 BE-P1-001(rollback 실패 정보 손실)이 발견돼 이 판정은 대체됐다. 수정 후 검증은 아래 기록을 따른다.


## BE-P1-001 수정 검증 (2026-10-04)

수정 전 `9a3a4d3`에서 일부 파일이 새 내용으로 남고 백업이 보존돼도 공개 코드가 file_commit_failed/cancelled로 축약되는 것을 재현했다. rollback 실패를 전용 typed error로 만들고 errors.Join의 다른 원인과 timeout/cancel보다 먼저 판별하도록 수정했다. 오류 문자열·unwrap은 기존 상세 원인을 유지한다.

- 실제 파일 교체 실패 주입: commit/cancel/deadline 원인 모두 file_rollback_failed 우선, 원본 백업 bytes·출력 순번·상대 복구 폴더 일치 확인.
- 신규 출력이 외부에서 바뀐 경우 해당 변경 보존, backup_file 생략과 rollback_failed 상태 확인. 정상 rollback은 원본 복원·staging 정리 및 기존 일반 오류 코드 유지.
- MCP 경계: 같은 typed error를 테스트 도구에서 반환해 실제 SDK session으로 직렬화, content와 `_meta`의 복구 정보/코드 일치·request ID·로그 코드·절대 경로 비노출 확인. Aseprite를 흉내 내는 실행 파일은 사용하지 않았다. 파일 실패 주입 검증과 MCP 직렬화 검증은 각각의 경계에서 수행했다.
- Linux build·vet·전체 unit/race 및 실제 Aseprite export/output/warnings/diagnostics/rollback 관련 integration 통과 (pkg/tools 8.011초). 마지막 정상 rollback 회귀 추가 후 output race 재검증 통과. macOS arm64의 output 및 MCP 오류 경계 회귀 통과. Lua 실행 코드는 이번 수정에서 변경하지 않아 최소 Aseprite 버전 재검증은 반복하지 않았다. 전체 integration 재실행 대신 영향 범위 통합 검증을 수행했다.

`convergent-code-review` 수정 라운드: 기존 base `7f439a0`, 수정 시작 HEAD `9a3a4d3`. 기존 40개 PR 변경 파일과 이번 복구 계약 변경을 합친 전체 범위를 재검토했다. 최초 재확인 1회, repair cycle 1회, repair-diff 1회, closure 1회, fresh 전체 1회 (review pass 4회). BE-P1-001은 VERIFIED, P1 1→0; 신규·재오픈·미해결 finding 0건. 이번 최종 후보 선정 뒤 의미 있는 변경/인증 무효화 0회. 권한/기본 응답과 복구 필요 상태의 계약을 대조하고 검증 완료로 종료했다.

Reviewed: 기존 OPS-03 PR 범위와 rollback error/복구 참조/분류 우선순위/회귀/문서. Context: output staging/backup과 MCP error conversion. Changed during repair: diagnostics와 output_files, 관련 테스트 및 문서. Excluded: 자동 복구, 새로운 복구 도구, plugin 저장소, 원래 파일 교체 알고리즘 변경. 복구 폴더는 원래 canonical 출력 부모 기준이며 경로 alias가 바뀐 경우 원래 위치의 수동 확인이 필요하다.

현재 최종 판정: PASS. 이전 BLOCKED 판정을 대체한다.

## Cel 편집 후속

Cel 편집의 `expected_revision` 불일치는 기존 `file_changed`를 사용한다. 입력 범위는 `invalid_arguments`, 잘못된 대상·locked·linked 동의 누락·변경 없음은 `lua_error`다. 새로운 공개 오류 코드/비정형 상세 payload를 추가하지 않는다. [CEL_PROPERTIES](CEL_PROPERTIES.md).

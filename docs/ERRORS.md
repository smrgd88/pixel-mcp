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
| `file_changed`, `file_commit_failed`, `file_lock_scope` | 파일 변경 감지, 교체 실패, 잠금 범위 오류 |
| `snapshot_invalid`, `snapshot_capacity`, `snapshot_expired`, `snapshot_source_mismatch` | snapshot 검증·한도·만료·원본 불일치 |
| `history_invalid`, `history_scope`, `history_conflict`, `history_operation_mismatch`, `history_recovery_required` | history 검증·범위·파일 변경·순서·미확정 전이 |
| `operation_failed` | 아직 세부 분류가 없는 도구 오류/명시적 error result |
| `protocol_error` | 그 밖의 tools/call 프로토콜 오류 |

분류는 typed error/unwrap 및 context·filesystem sentinel을 사용한다. 임의 오류 문자열에서 `history_conflict:` 등을 검색해서 코드로 승격하지 않는다. timeout/cancelled를 우선하고, 그다음 명시적 domain code를 적용한다. snapshot 만료가 lazy cleanup으로 먼저 삭제된 경우에는 `not_found`일 수 있다. Lua 안에서 검사한 잘못된 레이어/프레임 등의 조건은 `lua_error`이며 모든 의미 오류를 `invalid_arguments`로 추측하지 않는다.

Lua 실행은 원본 script와 작은 loader를 private 임시 파일로 작성한다. loader의 `loadfile`/`xpcall`이 구문·실행 오류에 호출별 무작위 marker를 붙인다. Aseprite의 stdout 또는 stderr에서 이번 marker를 확인해 Lua 오류를 구분하며 두 임시 파일 모두 정상 오류/취소 종료 시 정리한다. 프로세스가 Lua 진입 전에 실패한 경우는 일반 실행 오류다. capability probe 실패는 기존 capability 분류를 유지한다.

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

최종 판정: PASS.

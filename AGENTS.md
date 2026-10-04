# pixel-mcp 개발 지침

이 파일은 저장소 전체에 적용하는 Codex 작업 지침이다. 기존 `CLAUDE.md`의 프로젝트 설명을 바탕으로 현재 구현과 Git 운영 규칙에 맞췄다. 세부 계약은 아래 링크의 문서와 실제 코드를 확인한다. `README.original.md`는 과거 안내 백업이다.

## 작업 시작과 Git 운영

파일을 수정하기 전에 다음을 실행한다. 앱 화면의 브랜치 표시만 신뢰하지 않는다.

```bash
pwd
git rev-parse --show-toplevel
git branch --show-current
git status --short --branch
git worktree list --porcelain
```

- 실제 세션 경로·브랜치·워크트리가 예상과 다르면 수정 전에 예상값과 실제값을 사용자에게 보고한다.
- `main`은 운영 배포 기준, `develop`은 개발 통합 기준이다. 일반 작업은 최신 `develop`에서 전용 작업 브랜치를 만든다. `main`/`develop`에 직접 기능 커밋을 추가하지 않는다.
- 하나의 작업은 하나의 전용 브랜치와 전용 Codex/Orca 앱 관리 워크트리에서 수행한다. 별도의 임시 워크트리로 우회하지 않는다. 다른 세션과 브랜치·워크트리를 공유하지 않는다.
- 순서: 영역·유형 결정 → 저장소/워크트리 확인 → 기준 브랜치 확인 → 작업 브랜치 및 앱 관리 워크트리 생성/연결 → 실제 경로·브랜치 재확인 → 구현.
- FE와 BE 작업은 분리하고 큰 공통 변경은 SHARED로 분리한다. 할당된 범위만 수정한다.
- 다른 작업의 미커밋 변경을 발견하면 보존하고 보고한다. 브랜치 전환, stash, reset, clean, 파일/워크트리 삭제로 치우지 않는다.
- 테스트·리뷰가 끝난 작업만 `develop`에 통합한다. 배포 검증이 완료된 `develop`만 `main`에 반영한다. 병합은 사용자 지시에 포함되거나 별도 승인을 받은 경우에만 한다.
- 운영 긴급 수정은 `main`에서 hotfix 브랜치를 만들고 완료 후 `main`과 `develop` 모두에 반영한다. 이 경우에도 병합 승인 규칙을 따른다.
- 리뷰 전용 워크트리를 사용할 수 있지만 리뷰만을 위한 불필요한 브랜치는 만들지 않는다.
- 워크트리/브랜치는 병합 완료, 미커밋 변경 없음, 다른 세션 미사용을 모두 확인한 후에만 정리한다.

## 이름과 완료 보고

- 작업·세션: `[영역][작업유형] 작업 설명`.
- 영역: `FE`, `BE`, `SHARED`, `OPS`.
- 유형: `FEATURE`, `FIX`, `HOTFIX`, `REFACTOR`, `TEST`, `DOCS`, `CHORE`, `REVIEW`, `SPIKE`, `RELEASE`.
- 브랜치: `<유형>/<영역>-<작업명>`, 영문 소문자·하이픈. 예: `test/shared-regression-matrix`, `docs/shared-codex-readme`.
- 지정 가능한 워크트리 이름: `<영역>-<유형>-<작업명>`. 예: `shared-docs-codex-readme`.
- 기존 진행 중인 브랜치는 작업 도중 이름을 바꾸지 않고 신규 작업부터 규칙을 적용한다.
- 커밋에는 현재 작업 관련 변경만 포함한다. Conventional Commits 권장: `feat(be): ...`, `test(shared): ...`, `docs(shared): ...`.
- 완료 보고에는 작업명, 실제 워크트리 경로, 브랜치, 기준 브랜치·커밋, 주요 변경, 커밋 해시(없으면 미커밋), 검증 결과·미실행 이유, 남은 변경, 리뷰·병합 상태, 정리 가능 여부를 포함한다.

## 프로젝트와 실행 구조

Go로 작성한 로컬 MCP stdio 서버다. AI 클라이언트 요청을 Go handler에서 검증하고 Lua 스크립트를 생성하여 Aseprite CLI로 실행한다. Codex 중심으로 개발하되 MCP 클라이언트에 종속된 서버 동작을 가정하지 않는다.

- `cmd/pixel-mcp/`: CLI, 설정 로드, health, 로그 초기화.
- `pkg/config/`: JSON 설정과 경로 우선순위.
- `pkg/server/`: 도구 등록, MCP stdio, 공통 오류·요청 추적.
- `pkg/tools/`: 도구 schema/handler, warnings, 파일 보호, dry-run, snapshot/history 도구.
- `pkg/aseprite/`: 프로세스 실행, `lua_*.go` 생성기, 색상 알고리즘, 파일 보호·snapshot/history 저장.
- `internal/diagnostics/`: 오류 분류와 공개 오류 계약.
- `internal/testutil/`: 테스트 설정과 지원 함수.
- `examples/`: Go 클라이언트·감색·shading 예제.

편집 호출마다 별도의 `aseprite --batch` 프로세스를 사용한다. GUI의 열린 문서나 native undo stack이 MCP 호출 사이에 유지된다고 가정하지 않는다. 선택/클립보드와 복구 상태는 파일에 저장하므로 모든 상태가 없는 서버라고 설명하지 않는다.

## 설정과 지원 하한

Go 버전·의존성은 `go.mod`를 기준으로 한다. 현재 Go 1.25 이상, Aseprite 1.3.17.2 이상 및 `app.apiVersion >= 39`가 필요하다. 최소/검증 버전과 예외는 [CAPABILITIES](docs/CAPABILITIES.md)를 확인한다.

설정 파일 선택 순서:

1. `--config <path>`
2. `PIXEL_MCP_CONFIG`
3. `~/.config/pixel-mcp/config.json`

`aseprite_path`는 실제 실행 파일의 절대 경로로 명시한다. PATH 검색이나 설치 경로 자동 탐색을 추가하지 않는다. `PIXEL_MCP_CONFIG`는 설정 파일 선택용이며 Aseprite 실행 파일 경로를 직접 지정하는 환경 변수가 아니다.

```json
{
  "aseprite_path": "/absolute/path/to/aseprite",
  "temp_dir": "/tmp/pixel-mcp",
  "timeout": 30,
  "log_level": "info",
  "log_file": "",
  "enable_timing": false,
  "enable_history": false
}
```

- `timeout`은 초 단위이며 기본값 30이다. 실패한 테스트를 통과시키기 위해 무조건 늘리지 않는다.
- `log_file`이 비어 있으면 stderr 로그만 사용한다. stdout은 MCP 프로토콜 전용이다.
- `enable_history`는 자동 이력 기록의 opt-in이며 기본 false다.
- `snapshot_dir`를 지정하면 절대 경로여야 한다. 기본 저장소는 `os.UserConfigDir()/pixel-mcp/snapshots`이고 `temp_dir`와 분리된다.
- 테스트 설정은 임시 경로에 두고 `PIXEL_MCP_CONFIG`로 선택한다. 사용자 기본 설정을 덮어쓰지 않는다.

## 빌드와 검증

저장소 루트에서 필요한 명령을 선택한다.

```bash
go build -o bin/pixel-mcp ./cmd/pixel-mcp
go vet ./...
go test ./pkg/config
```

실제 Aseprite가 필요한 검사에는 별도의 설정을 준비한다.

```bash
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -race -cover ./...
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -tags=integration ./...
./bin/pixel-mcp --config /tmp/pixel-mcp-test-config.json --health
```

- `make build`, `make test`, `make test-integration`, `make test-coverage`도 제공한다. `make lint`는 `go vet`와 `go fmt`를 실행하므로 파일이 변경될 수 있다.
- 순수 Go 단위 테스트는 Aseprite나 사용자 전역 설정이 필요하지 않을 수 있다. Aseprite 동작/통합 검증에는 실제 실행 파일을 사용하고 mock 결과로 대체하지 않는다.
- [TESTING](docs/TESTING.md)의 Docker 실행을 사용할 수 있다. 병렬 Aseprite 작업으로 인한 timeout을 피하도록 전체 검증은 순차 실행한다.
- 코드 변경 범위에 맞게 컴파일·단위 테스트·관련 PBT·통합·정적 분석을 실행한다. 파일 편집 도구는 실제 저장/재열기·렌더링 및 실패 시 원본 보존을 확인한다. 성공 응답만으로 정확성을 판단하지 않는다.
- macOS 개발에서는 Linux CI와 필요한 macOS 관련 회귀를 확인한다. 지원 하한 전체 검증은 릴리스 직전 또는 명시 요청이 없으면 반복하지 않는다.
- Windows 네이티브 테스트는 현재 개발 범위에서 제외한다. 반복적인 미실행 경고나 진행 차단 사유로 쓰지 않는다. 해당 환경 작업 또는 명시 요청 시 범위를 재설정한다.
- 문서만 변경하면 링크·예제 구문·코드와의 일치 여부를 확인한다. 실행하지 않은 코드 테스트/클라이언트 UI 검증을 통과했다고 보고하지 않는다.

## 코드와 데이터 보호

- Go 표준 관례, `gofmt`, exported API 주석, table-driven test를 따른다. 오류는 맥락을 보존해 wrapping하고 공개 응답은 [ERRORS](docs/ERRORS.md) 계약을 따른다.
- Lua 입력은 `EscapeString` 등 기존 안전한 직렬화 경로를 사용한다. null/빈 객체, 문자열 quoting과 좌표 기준을 보존한다.
- 한 Lua 실행의 변경은 적절한 `app.transaction()`으로 묶고 저장 결과를 검증한다. transaction을 호출 간 복구나 파일 저장의 crash 원자성으로 설명하지 않는다.
- 같은 파일의 잠금, staging, atomic 교체, 취소/실패 보호는 기존 공통 경로를 사용한다. 다중 export의 일반 오류 rollback과 프로세스/전원 중단 복구 보장을 구분한다.
- 선택 mask는 `spr.properties("pixel-mcp/selection")`에 저장한다. 레거시 `sprite.data`는 읽을 수 있지만 사용자 데이터를 다시 쓰지 않는다.
- 클립보드는 같은 sprite의 숨김 `__mcp_clipboard__` 레이어이며 OS 클립보드가 아니다.
- cel의 로컬 좌표와 sprite 절대 좌표를 구분한다. indexed 픽셀은 RGBA가 아닌 palette index이며 transparent index를 보존한다.
- native linked cel의 image 공유, occupied target 거부, 마지막 layer/frame 삭제 보호를 유지한다.
- `use_palette` 등 옵션의 적용 범위를 개별 schema/handler에서 확인한다. 특정 도구의 지원을 전체 도구에 일반화하지 않는다.
- 도구의 이름·필수 입력·성공 payload·warnings 호환성을 보존한다. 변경이 필요하면 호환성 영향을 명시한다.

## 작업 목록과 문서의 기준

- [NEXT_STEPS](docs/NEXT_STEPS.md): 실행 체크리스트와 검증 이력.
- [ROADMAP](docs/ROADMAP.md): 우선순위·단계·의존성. GAP 번호를 착수 순서로 해석하지 않는다.
- [CAPABILITIES](docs/CAPABILITIES.md): 실제 지원 상태와 GAP/SAFE/OPS ID.
- [FILE_PROTECTION](docs/FILE_PROTECTION.md), [DRY_RUN](docs/DRY_RUN.md), [SNAPSHOTS](docs/SNAPSHOTS.md), [HISTORY](docs/HISTORY.md), [WARNINGS](docs/WARNINGS.md), [ERRORS](docs/ERRORS.md): 동작·제외 범위.
- 기능 변경 시 관련 지원표와 CHANGELOG의 Unreleased를 갱신한다. 등록 도구 수는 실제 `Register*Tools`와 대조한다.
- 문서의 진행 상태가 오래됐으면 실제 커밋·PR 상태와 대조한다. develop 반영, 태그 포함, 배포 완료를 구분한다.
- README 사용자 안내는 한국어·영어를 함께 유지한다. Codex 연결 설정과 서버 JSON 설정을 구분한다.

Codex의 지침 검색 방식은 [공식 AGENTS.md 안내](https://developers.openai.com/codex/guides/agents-md)를 참고한다.

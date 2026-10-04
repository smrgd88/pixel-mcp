# pixel-mcp contributor instructions

This file contains the repository-wide instructions Codex discovers and follows.

These instructions adapt the project guidance in `CLAUDE.md` to the current implementation and Git workflow. Consult the linked contracts and source code for details. `README.original.md` is a historical backup.

## Before editing and Git workflow

Run the following before editing files. Do not rely solely on the branch label shown in the app.

```bash
pwd
git rev-parse --show-toplevel
git branch --show-current
git status --short --branch
git worktree list --porcelain
```

- If the session path, branch, or worktree differs from the expected state, report the expected and actual values before editing.
- `main` is the production baseline; `develop` is the integration baseline. Create a dedicated task branch from the latest `develop` for ordinary work. Do not add feature commits directly to `main` or `develop`.
- Use one dedicated branch and one Codex/Orca app-managed worktree per task. Do not bypass this with a separate temporary worktree. Do not share branches or worktrees between sessions.
- Sequence: choose area/type → inspect repository/worktree → verify base branch → create/connect task branch and app-managed worktree → recheck actual path/branch → implement.
- Separate FE and BE tasks. Split substantial shared changes into a SHARED task. Modify only the assigned scope.
- Preserve and report uncommitted changes belonging to another task. Do not clear them through branch switching, stash, reset, clean, or file/worktree deletion.
- Integrate into `develop` only after tests and review. Promote `develop` to `main` only after release validation. Merge only when the user requested it as part of the task or separately approved it.
- For production emergencies, create a hotfix branch from `main` and apply the completed fix to both `main` and `develop`. The same merge-approval rule applies.
- A separate review worktree is allowed; do not create an unnecessary branch solely for review.
- Remove a worktree/branch only after confirming it is merged, has no uncommitted changes, and is not used by another session.

## Naming and completion reports

- Task/session: `[AREA][TYPE] Description`.
- Areas: `FE`, `BE`, `SHARED`, `OPS`.
- Types: `FEATURE`, `FIX`, `HOTFIX`, `REFACTOR`, `TEST`, `DOCS`, `CHORE`, `REVIEW`, `SPIKE`, `RELEASE`.
- Branch: `<type>/<area>-<task-name>`, using lowercase English and hyphens. Examples: `test/shared-regression-matrix`, `docs/shared-codex-readme`.
- Worktree name, when configurable: `<area>-<type>-<task-name>`. Example: `shared-docs-codex-readme`.
- Do not rename an existing in-progress branch mid-task. Apply the convention to new work.
- Commit only changes related to the current task. Prefer Conventional Commits: `feat(be): ...`, `test(shared): ...`, `docs(shared): ...`.
- Report the task name, actual worktree path, branch, base branch/commit, main changes, commit hash (or uncommitted status), validation results and reasons for skipped checks, remaining changes, review/merge status, and cleanup eligibility.

## Project and execution model

This is a local MCP stdio server written in Go. Go handlers validate AI client requests and generate Lua scripts for the Aseprite CLI. Development centers on Codex, but do not assume client-specific server behavior.

- `cmd/pixel-mcp/`: CLI, config loading, health, and logging initialization.
- `pkg/config/`: JSON configuration and path precedence.
- `pkg/server/`: tool registration, MCP stdio, shared errors and request tracing.
- `pkg/tools/`: tool schemas/handlers, warnings, file protection, dry-run, snapshot/history tools.
- `pkg/aseprite/`: process execution, `lua_*.go` generators, color algorithms, file protection and snapshot/history storage.
- `internal/diagnostics/`: error classification and public error contracts.
- `internal/testutil/`: test configuration and helpers.
- `examples/`: Go client, quantization, and shading examples.

Each editing call uses a separate `aseprite --batch` process. Do not assume an open GUI document or native undo stack persists across MCP calls. Selection/clipboard and recovery state are stored in files; do not describe the server as having no persistent state at all.

## Configuration and minimum versions

Use `go.mod` for Go and dependency versions. Current requirements are Go 1.25+, Aseprite 1.3.17.2+, and `app.apiVersion >= 39`. See [CAPABILITIES](docs/CAPABILITIES.md) for minimum/tested versions and exceptions.

Configuration file precedence:

1. `--config <path>`
2. `PIXEL_MCP_CONFIG`
3. `~/.config/pixel-mcp/config.json`

Set `aseprite_path` explicitly to the absolute path of a real executable. Do not add PATH searches or installation auto-discovery. `PIXEL_MCP_CONFIG` selects the configuration file; it does not directly specify the Aseprite executable.

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

- `timeout` is in seconds and defaults to 30. Do not increase it indiscriminately just to make failing tests pass.
- An empty `log_file` means stderr logging only. stdout is reserved for the MCP protocol.
- `enable_history` opts into automatic history recording and defaults to false.
- If supplied, `snapshot_dir` must be absolute. The default store is `os.UserConfigDir()/pixel-mcp/snapshots`, separate from `temp_dir`.
- Put test configuration in a temporary location and select it with `PIXEL_MCP_CONFIG`. Do not overwrite the user's default configuration.

## Build and validation

Select the relevant commands and run them from the repository root.

```bash
go build -o bin/pixel-mcp ./cmd/pixel-mcp
go vet ./...
go test ./pkg/config
```

Prepare a separate configuration for checks that need real Aseprite.

```bash
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -race -cover ./...
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -tags=integration ./...
./bin/pixel-mcp --config /tmp/pixel-mcp-test-config.json --health
```

- `make build`, `make test`, `make test-integration`, and `make test-coverage` are also available. `make lint` runs `go vet` and `go fmt`, so it can modify files.
- Pure Go unit tests may need neither Aseprite nor user-level configuration. Use a real executable for Aseprite behavior/integration validation; do not substitute mock results.
- Docker execution is documented in [TESTING](docs/TESTING.md). Run full validation sequentially to avoid timeouts from concurrent Aseprite workloads.
- Run compilation, unit tests, relevant property-based tests, integration tests, and static analysis appropriate to the code change. For editing tools, verify actual save/reopen, rendered results, and original-file preservation on failure. A success response alone does not establish correctness.
- During macOS development, check Linux CI and relevant macOS regressions. Do not repeat the full minimum-version suite except before release or when explicitly requested.
- Native Windows testing is outside the current development scope. Do not repeatedly report it as a skipped-check warning or blocker. Revisit scope for work on that environment or an explicit request.
- For documentation-only changes, check links, example syntax, and agreement with the code. Do not claim code tests or client UI validation passed if they were not run.

## Code and data protection

- Follow Go conventions, `gofmt`, exported API comments, and table-driven tests. Wrap errors with context and follow [ERRORS](docs/ERRORS.md) for public responses.
- Use existing safe serialization paths such as `EscapeString` for Lua input. Preserve null/empty-object semantics, string quoting, and coordinate conventions.
- Wrap changes within a Lua execution in an appropriate `app.transaction()` and verify saving. Do not describe a transaction as cross-call recovery or crash-atomic file saving.
- Reuse shared paths for per-file locks, staging, atomic replacement, and cancellation/failure protection. Distinguish ordinary multi-output export rollback from process/power-loss recovery guarantees.
- Store selection masks in `spr.properties("pixel-mcp/selection")`. Legacy `sprite.data` can be read, but do not rewrite user data.
- The clipboard is the hidden `__mcp_clipboard__` layer in the same sprite, not the OS clipboard.
- Distinguish cel-local from sprite-absolute coordinates. Indexed pixels contain palette indices rather than RGBA values; preserve the transparent index.
- Preserve native linked-cel image sharing, rejection of occupied targets, and protection against deleting the last layer/frame.
- Check each schema/handler for options such as `use_palette`. Do not generalize one tool's support to all tools.
- Preserve tool names, required inputs, successful payloads, and warnings compatibility. State compatibility effects when changes are needed.

## Task and documentation sources

- [NEXT_STEPS](docs/NEXT_STEPS.md): execution checklist and validation history.
- [ROADMAP](docs/ROADMAP.md): priorities, stages, and dependencies. GAP numbers are not execution order.
- [CAPABILITIES](docs/CAPABILITIES.md): actual support status and GAP/SAFE/OPS IDs.
- [FILE_PROTECTION](docs/FILE_PROTECTION.md), [DRY_RUN](docs/DRY_RUN.md), [SNAPSHOTS](docs/SNAPSHOTS.md), [HISTORY](docs/HISTORY.md), [WARNINGS](docs/WARNINGS.md), [ERRORS](docs/ERRORS.md): behavior and exclusions.
- For feature changes, update the relevant capability inventory and CHANGELOG Unreleased section. Cross-check tool counts against actual `Register*Tools` implementations.
- If documented progress is stale, compare it with actual commits and PR status. Distinguish develop integration, inclusion in a tag, and completed deployment.
- Keep Korean and English README guidance synchronized. Distinguish Codex connection configuration from the server's JSON configuration.

See the [official AGENTS.md guide](https://developers.openai.com/codex/guides/agents-md) for Codex instruction discovery.

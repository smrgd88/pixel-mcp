# Testing Guide

## Prerequisites

Pure unit tests do not require a user-level configuration file. Tests that execute
Aseprite require a real installation and a configuration file. The test configuration
can live anywhere; setting `PIXEL_MCP_CONFIG` keeps it isolated from the user's default.

Create a test configuration file, for example `/tmp/pixel-mcp-test-config.json`:

**macOS:**
```json
{
  "aseprite_path": "/Applications/Aseprite.app/Contents/MacOS/aseprite",
  "temp_dir": "/tmp/pixel-mcp",
  "timeout": 30,
  "log_level": "info",
  "log_file": "",
  "enable_timing": false
}
```

**Linux:**
```json
{
  "aseprite_path": "/usr/bin/aseprite",
  "temp_dir": "/tmp/pixel-mcp",
  "timeout": 30,
  "log_level": "info",
  "log_file": "",
  "enable_timing": false
}
```

**Windows:**
```json
{
  "aseprite_path": "C:\\Program Files\\Aseprite\\aseprite.exe",
  "temp_dir": "C:\\Temp\\pixel-mcp",
  "timeout": 30,
  "log_level": "info",
  "log_file": "",
  "enable_timing": false
}
```

## Unit Tests

Run pure configuration unit tests without Aseprite or a global configuration:
```bash
go test ./pkg/config
```

To run the full suite, select the isolated test configuration:
```bash
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test ./...
```

Run with verbose output:
```bash
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -v ./...
```

## Integration Tests

Run integration tests (requires config file with real Aseprite):
```bash
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -tags=integration ./...
```

Run integration tests with verbose output:
```bash
PIXEL_MCP_CONFIG=/tmp/pixel-mcp-test-config.json go test -tags=integration -v ./pkg/aseprite
```

## Coverage

Generate coverage report:
```bash
make test-coverage
open coverage.html
```

## Test Structure

- **Unit tests**: `*_test.go` files (no build tags)
  - Test pure Go logic, Lua script generation, string escaping
  - Configuration package tests use temporary files and do not need Aseprite
  - Some behavior tests execute a real Aseprite process

- **Integration tests**: `integration_test.go` files with `//go:build integration`
  - Test actual Aseprite execution
  - Create real sprites, draw pixels, export images
  - Verify file I/O with real Aseprite binary

## Docker Testing

Run tests in Docker CI environment (includes Aseprite):
```bash
make docker-test-all
```

This runs both unit and integration tests in the CI container with Aseprite pre-built.
The Docker test helpers create `/tmp/pixel-mcp-ci-config.json` inside the disposable
container and select it with `PIXEL_MCP_CONFIG`; they do not read or write a user-level
`~/.config/pixel-mcp/config.json`.

Build the supported Aseprite version images explicitly:
```bash
docker build -f Dockerfile.ci --build-arg ASEPRITE_VERSION=v1.3.17.2 -t pixel-mcp-ci:aseprite-1.3.17.2 .
docker build -f Dockerfile.ci --build-arg ASEPRITE_VERSION=v1.3.18.3 -t pixel-mcp-ci:aseprite-1.3.18.3 .
```

For `link_cel`, integration coverage verifies native image identity after save/reopen,
pixel propagation in both directions, cel position preservation, invalid inputs, occupied
targets, and original-file preservation on failure.

## Manual Testing

Test server manually:
```bash
go run ./cmd/pixel-mcp --config /tmp/pixel-mcp-test-config.json

# Validate the same isolated configuration and Aseprite installation
go run ./cmd/pixel-mcp --config /tmp/pixel-mcp-test-config.json --health
```

Test server via Docker:
```bash
make docker-run-full
```

## Testing Philosophy

Integration and Aseprite behavior tests use a real executable to ensure:
- Tests accurately reflect Aseprite's actual behavior
- Changes in Aseprite's API are detected immediately
- Integration issues are discovered during development
- High confidence that the server works correctly in production

## Troubleshooting

**Error: "config file not found"**
- Pass `--config <path>`, set `PIXEL_MCP_CONFIG`, or create
  `~/.config/pixel-mcp/config.json`

**Error: "aseprite executable not found"**
- Verify the path in config.json points to a real Aseprite binary
- On Windows, use double backslashes: `D:\\path\\to\\aseprite.exe`

**Tests timeout**
- Increase `timeout` value in config.json (value in seconds)
- Default is 30 seconds

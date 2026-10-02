package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateLoggerDefaultRedactionAndDebugOptIn(t *testing.T) {
	for _, level := range []string{"info", "debug"} {
		t.Run(level, func(t *testing.T) {
			dir := t.TempDir()
			logger := createLogger(level, filepath.Join(dir, "server.log"))
			logger.Information("Request {RequestID} path {Path}", "request-123", "private-test-payload")
			closeLogger(logger)
			files, err := filepath.Glob(filepath.Join(dir, "*"))
			require.NoError(t, err)
			require.NotEmpty(t, files)
			var content strings.Builder
			for _, path := range files {
				b, err := os.ReadFile(path)
				require.NoError(t, err)
				content.Write(b)
			}
			require.Contains(t, content.String(), "request-123")
			if level == "info" {
				require.NotContains(t, content.String(), "private-test-payload")
			} else {
				require.Contains(t, content.String(), "private-test-payload")
			}
		})
	}
}

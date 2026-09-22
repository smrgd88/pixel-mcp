package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
)

func TestFileProtectionSheetMetadataLock(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.aseprite")
	texture := filepath.Join(dir, "sheet.png")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	ready, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- aseprite.WithFileLocks(context.Background(), []string{filepath.Join(dir, "sheet.json")}, func(context.Context) error { close(ready); <-release; return nil })
	}()
	<-ready
	defer func() { close(release); require.NoError(t, <-done) }()
	called := false
	handler := wrapWithFileProtection("export_spritesheet", 30*time.Millisecond, func(context.Context, *mcp.CallToolRequest, ExportSpritesheetInput) (*mcp.CallToolResult, *ExportSpritesheetOutput, error) {
		called = true
		return nil, &ExportSpritesheetOutput{}, nil
	})
	_, _, err := handler(context.Background(), nil, ExportSpritesheetInput{SpritePath: source, OutputPath: texture, IncludeJSON: false})
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, called)
}

func TestFileProtectionSheetOutputAliases(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.aseprite")
	texture := filepath.Join(dir, "sheet.png")
	metadata := filepath.Join(dir, "sheet.json")
	require.NoError(t, os.WriteFile(source, []byte("source"), 0600))
	require.NoError(t, os.WriteFile(texture, []byte("texture"), 0600))
	require.NoError(t, os.Link(texture, metadata))
	require.ErrorContains(t, validateSheetOutputs(source, []string{texture, metadata}), "alias")
	require.ErrorContains(t, validateSheetOutputs(source, []string{texture, texture}), "alias")
	require.NoError(t, os.Remove(metadata))
	require.NoError(t, validateSheetOutputs(source, []string{texture, metadata}))
}

package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

type ListOperationHistoryInput struct {
	SpritePath string `json:"sprite_path" jsonschema:"Original sprite path whose confirmed edit history to list"`
}
type ListOperationHistoryOutput struct {
	Operations       []aseprite.HistoryEntry `json:"operations"`
	RecordingEnabled bool                    `json:"recording_enabled"`
}
type UndoLastOperationInput struct {
	SpritePath          string `json:"sprite_path" jsonschema:"Existing writable original sprite"`
	ExpectedOperationID string `json:"expected_operation_id" jsonschema:"Latest applied operation_id from list_operation_history; guards retries and concurrent edits"`
}
type UndoLastOperationOutput struct {
	Success        bool                  `json:"success"`
	Operation      aseprite.HistoryEntry `json:"operation"`
	BackupSnapshot aseprite.Snapshot     `json:"backup_snapshot"`
}

// RegisterHistoryTools exposes recovery even when automatic recording is disabled.
func RegisterHistoryTools(server *mcp.Server, cfg *config.Config, logger core.Logger) {
	store := aseprite.NewSnapshotStore(cfg.SnapshotDir)
	mcp.AddTool(server, &mcp.Tool{Name: "list_operation_history", Description: "List confirmed saved-file edits newest first, including undone entries. Automatic recording requires enable_history=true. History expires or disappears when its snapshot is expired or deleted. May reconcile a pending journal without modifying the sprite."}, snapshotHandler("list_operation_history", cfg, logger, func(ctx context.Context, in ListOperationHistoryInput) (*ListOperationHistoryOutput, error) {
		entries, err := store.ListHistory(ctx, in.SpritePath)
		if err != nil {
			return nil, err
		}
		return &ListOperationHistoryOutput{entries, cfg.EnableHistory}, nil
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "undo_last_operation", Description: "Undo the latest still-applied recorded edit by expected operation ID. Refuses external changes, missing/expired snapshots, and stale IDs. Creates a pre-undo snapshot subject to the same capacity and expiry policy; only saved-file changes are included."}, snapshotHandler("undo_last_operation", cfg, logger, func(ctx context.Context, in UndoLastOperationInput) (*UndoLastOperationOutput, error) {
		entry, backup, err := store.UndoLast(ctx, in.SpritePath, in.ExpectedOperationID)
		if err != nil {
			return nil, err
		}
		return &UndoLastOperationOutput{true, entry, backup}, nil
	}))
}

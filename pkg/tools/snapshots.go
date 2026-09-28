package tools

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/willibrandon/mtlog/core"
	"github.com/willibrandon/pixel-mcp/pkg/aseprite"
	"github.com/willibrandon/pixel-mcp/pkg/config"
)

type CreateSnapshotInput struct {
	SpritePath string `json:"sprite_path" jsonschema:"Path of the saved sprite to copy byte-for-byte; unsaved editor changes are not included"`
	Label      string `json:"label,omitempty" jsonschema:"Optional label, at most 256 UTF-8 bytes"`
}
type CreateSnapshotOutput struct {
	Success  bool              `json:"success"`
	Snapshot aseprite.Snapshot `json:"snapshot"`
}
type ListSnapshotsInput struct {
	SpritePath string `json:"sprite_path,omitempty" jsonschema:"Optional original sprite path filter; omit to list all unexpired snapshots"`
}
type ListSnapshotsOutput struct {
	Snapshots []aseprite.Snapshot `json:"snapshots"`
}
type RestoreSnapshotInput struct {
	SpritePath string `json:"sprite_path" jsonschema:"Existing writable original sprite path; must match the snapshot source"`
	SnapshotID string `json:"snapshot_id" jsonschema:"UUID returned by create_snapshot or list_snapshots"`
}
type RestoreSnapshotOutput struct {
	Success        bool              `json:"success"`
	SnapshotID     string            `json:"snapshot_id"`
	BackupSnapshot aseprite.Snapshot `json:"backup_snapshot" jsonschema:"Pre-restore bytes, subject to the same expiry and storage limits"`
}
type DeleteSnapshotInput struct {
	SnapshotID string `json:"snapshot_id" jsonschema:"UUID of the snapshot to permanently delete"`
}
type DeleteSnapshotOutput struct {
	Success    bool   `json:"success"`
	SnapshotID string `json:"snapshot_id"`
}

// Snapshot operations own their complete lock set; they must not pass through
// the generic sprite-edit wrapper, which would bind or publish an extra copy.
func snapshotHandler[I, O any](name string, cfg *config.Config, logger core.Logger, fn func(context.Context, I) (O, error)) func(context.Context, *mcp.CallToolRequest, I) (*mcp.CallToolResult, O, error) {
	handler := func(ctx context.Context, _ *mcp.CallToolRequest, in I) (*mcp.CallToolResult, O, error) {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		out, err := fn(ctx, in)
		return nil, out, err
	}
	if cfg.EnableTiming {
		return wrapWithTiming(name, logger, handler)
	}
	return handler
}

// RegisterSnapshotTools adds the four persistent recovery operations.
func RegisterSnapshotTools(server *mcp.Server, cfg *config.Config, logger core.Logger) {
	store := aseprite.NewSnapshotStore(cfg.SnapshotDir)
	mcp.AddTool(server, &mcp.Tool{Name: "create_snapshot", Description: "Save a byte-exact recovery copy of a saved sprite. Persists across server restarts for 7 days; store limit 100 snapshots / 512 MiB. Does not capture unsaved editor changes."}, snapshotHandler("create_snapshot", cfg, logger, func(ctx context.Context, in CreateSnapshotInput) (*CreateSnapshotOutput, error) {
		item, err := store.Create(ctx, in.SpritePath, in.Label)
		if err != nil {
			return nil, err
		}
		return &CreateSnapshotOutput{true, item}, nil
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "list_snapshots", Description: "List unexpired recovery copies, oldest first. Performs lazy expiry cleanup. Optionally filter by original sprite path."}, snapshotHandler("list_snapshots", cfg, logger, func(ctx context.Context, in ListSnapshotsInput) (*ListSnapshotsOutput, error) {
		items, err := store.List(ctx, in.SpritePath)
		if err != nil {
			return nil, err
		}
		return &ListSnapshotsOutput{items}, nil
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "restore_snapshot", Description: "Replace the existing original sprite with verified snapshot bytes using atomic replacement. First creates a recovery snapshot of the current file; fails without replacing the source if backup capacity is unavailable. Unsaved editor changes are not included."}, snapshotHandler("restore_snapshot", cfg, logger, func(ctx context.Context, in RestoreSnapshotInput) (*RestoreSnapshotOutput, error) {
		backup, err := store.Restore(ctx, in.SpritePath, in.SnapshotID)
		if err != nil {
			return nil, err
		}
		return &RestoreSnapshotOutput{true, in.SnapshotID, backup}, nil
	}))
	mcp.AddTool(server, &mcp.Tool{Name: "delete_snapshot", Description: "Permanently delete one recovery copy by UUID. Does not change the original sprite. Unknown IDs return an error."}, snapshotHandler("delete_snapshot", cfg, logger, func(ctx context.Context, in DeleteSnapshotInput) (*DeleteSnapshotOutput, error) {
		if err := store.Delete(ctx, in.SnapshotID); err != nil {
			return nil, err
		}
		return &DeleteSnapshotOutput{true, in.SnapshotID}, nil
	}))
}

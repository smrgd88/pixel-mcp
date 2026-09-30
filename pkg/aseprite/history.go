package aseprite

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

// OperationRecord contains only a tool identifier and recovery evidence, never
// request arguments, paths, labels, Lua, or image content. Snapshot metadata
// separately retains the source path needed for recovery.
type OperationRecord struct {
	ID           string    `json:"operation_id"`
	Tool         string    `json:"tool"`
	Sequence     int64     `json:"sequence"`
	CreatedAt    time.Time `json:"created_at"`
	State        string    `json:"state"`
	BeforeSHA256 string    `json:"before_sha256"`
	AfterSHA256  string    `json:"after_sha256"`
}

type HistoryEntry struct {
	OperationRecord
	SnapshotID string    `json:"snapshot_id"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// ReservationPath allows callers with additional input files to reserve the
// store together with all sprite/input locks in one globally ordered set.
func (s *SnapshotStore) ReservationPath() (string, error) { return s.root() }

func validOperation(op *OperationRecord) bool {
	if op == nil {
		return true
	}
	if !validSnapshotID(op.ID) || op.Tool == "" || len(op.Tool) > 80 || op.Sequence <= 0 || op.CreatedAt.IsZero() {
		return false
	}
	for _, h := range []string{op.BeforeSHA256, op.AfterSHA256} {
		b, err := hex.DecodeString(h)
		if err != nil || len(b) != 32 {
			return false
		}
	}
	switch op.State {
	case "pending", "applied", "undo_pending", "undone":
		return true
	}
	return false
}

func writeSnapshotMetadata(ctx context.Context, root string, m Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Join(root, m.ID)
	if err := privateSnapshotPath(dir, true); err != nil {
		return err
	}
	if err := privateSnapshotPath(filepath.Join(dir, "metadata.json"), false); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if len(data) > 16384 {
		return fmt.Errorf("history_invalid: metadata too large")
	}
	path := filepath.Join(dir, ".metadata-"+uuid.NewString())
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return replaceFile(path, filepath.Join(dir, "metadata.json"))
}

// recoverHistory only resolves a pending transition when the file is exactly
// one of its recorded endpoints. Ambiguous external edits are never overwritten.
func (s *SnapshotStore) recoverHistory(ctx context.Context, root, source string, entries []Snapshot) error {
	for _, m := range entries {
		op := m.Operation
		if op == nil || pathKey(m.SpritePath) != pathKey(source) || (op.State != "pending" && op.State != "undo_pending") {
			continue
		}
		current, err := snapshotSource(ctx, source, s.MaxBytes)
		if err != nil {
			return fmt.Errorf("history_recovery_required: %w", err)
		}
		hash := hex.EncodeToString(current.hash[:])
		if op.State == "pending" {
			switch hash {
			case op.AfterSHA256:
				op.State = "applied"
			case op.BeforeSHA256:
				m.Operation = nil
			default:
				return fmt.Errorf("history_recovery_required: current file matches neither endpoint")
			}
		} else {
			switch hash {
			case op.BeforeSHA256:
				op.State = "undone"
			case op.AfterSHA256:
				op.State = "applied"
			default:
				return fmt.Errorf("history_recovery_required: current file matches neither undo endpoint")
			}
		}
		if err := writeSnapshotMetadata(ctx, root, m); err != nil {
			return err
		}
	}
	return nil
}

// WithOperation runs a single-file edit on the existing staged sprite binding.
// A pending journal is persisted BEFORE the atomic file publication. A final
// metadata-write failure leaves a recoverable pending record, not an untracked
// successful edit. Ordinary failed edits never appear in successful history.
func (s *SnapshotStore) WithOperation(ctx context.Context, path, tool string, fn func(context.Context) error) error {
	if tool == "" || len(tool) > 80 {
		return fmt.Errorf("history_invalid: invalid tool identifier")
	}
	return s.run(ctx, path, func(ctx context.Context, root, source string) error {
		if _, bound, err := boundSprite(ctx, source); err != nil {
			return err
		} else if bound {
			return fmt.Errorf("history_scope: source already bound")
		}
		entries, err := s.inventory(ctx, root)
		if err != nil {
			return err
		}
		if err = s.recoverHistory(ctx, root, source, entries); err != nil {
			return err
		}
		entries, err = s.inventory(ctx, root)
		if err != nil {
			return err
		}
		sequence := int64(1)
		for _, m := range entries {
			if m.Operation != nil && m.Operation.Sequence >= sequence {
				sequence = m.Operation.Sequence + 1
			}
		}
		before, err := snapshotSource(ctx, source, s.MaxBytes)
		if err != nil {
			return err
		}
		var saved Snapshot
		err = WithSpriteAccess(ctx, source, true, func(ctx context.Context) error {
			if err := fn(ctx); err != nil {
				return err
			}
			staged, _, err := boundSprite(ctx, source)
			if err != nil {
				return err
			}
			after, err := snapshotSource(ctx, staged, s.MaxBytes)
			if err != nil {
				return err
			}
			if before.hash == after.hash {
				return nil
			}
			saved, err = s.create(ctx, root, source, "operation recovery", entries)
			if err != nil {
				return err
			}
			if saved.SHA256 != hex.EncodeToString(before.hash[:]) {
				return fmt.Errorf("file_changed: source changed before history backup")
			}
			saved.Operation = &OperationRecord{ID: uuid.NewString(), Tool: tool, Sequence: sequence, CreatedAt: time.Now().UTC(), State: "pending", BeforeSHA256: saved.SHA256, AfterSHA256: hex.EncodeToString(after.hash[:])}
			return writeSnapshotMetadata(ctx, root, saved)
		})
		if err != nil {
			if saved.ID != "" {
				_ = os.RemoveAll(filepath.Join(root, saved.ID))
			}
			return err
		}
		if saved.Operation != nil {
			saved.Operation.State = "applied"
			// Publication is already successful. The persisted pending transition is
			// sufficient to reconcile a late cancellation or storage failure next time.
			_ = writeSnapshotMetadata(ctx, root, saved)
		}
		return nil
	})
}

// ListHistory returns only confirmed edits for one sprite, newest first. Expiry
// and explicit snapshot deletion also remove the corresponding history entry.
func (s *SnapshotStore) ListHistory(ctx context.Context, path string) ([]HistoryEntry, error) {
	out := []HistoryEntry{}
	if path == "" {
		return out, fmt.Errorf("history_invalid: sprite_path required")
	}
	err := s.run(ctx, path, func(ctx context.Context, root, source string) error {
		entries, err := s.inventory(ctx, root)
		if err != nil {
			return err
		}
		if err = s.recoverHistory(ctx, root, source, entries); err != nil {
			return err
		}
		entries, err = s.inventory(ctx, root)
		if err != nil {
			return err
		}
		for _, m := range entries {
			if m.Operation != nil && pathKey(m.SpritePath) == pathKey(source) {
				out = append(out, HistoryEntry{*m.Operation, m.ID, m.ExpiresAt})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Sequence > out[j].Sequence })
		return nil
	})
	return out, err
}

// UndoLast restores the latest still-applied edit only when its expected ID and
// post-edit hash match. The ID prevents retried requests from undoing an older
// operation. Restoration retains SAFE-03's mandatory current-file backup.
func (s *SnapshotStore) UndoLast(ctx context.Context, path, expectedID string) (HistoryEntry, Snapshot, error) {
	var entry HistoryEntry
	var backup Snapshot
	if path == "" || !validSnapshotID(expectedID) {
		return entry, backup, fmt.Errorf("history_invalid: sprite_path and expected_operation_id required")
	}
	err := s.run(ctx, path, func(ctx context.Context, root, source string) error {
		entries, err := s.inventory(ctx, root)
		if err != nil {
			return err
		}
		if err = s.recoverHistory(ctx, root, source, entries); err != nil {
			return err
		}
		entries, err = s.inventory(ctx, root)
		if err != nil {
			return err
		}
		var latest Snapshot
		for _, m := range entries {
			if m.Operation != nil && pathKey(m.SpritePath) == pathKey(source) && m.Operation.State == "applied" && (latest.Operation == nil || m.Operation.Sequence > latest.Operation.Sequence) {
				latest = m
			}
		}
		if latest.Operation == nil || latest.Operation.ID != expectedID {
			return fmt.Errorf("history_operation_mismatch: no matching latest operation; refresh history")
		}
		current, err := snapshotSource(ctx, source, s.MaxBytes)
		if err != nil {
			return err
		}
		if hex.EncodeToString(current.hash[:]) != latest.Operation.AfterSHA256 {
			return fmt.Errorf("history_conflict: file changed after recorded operation")
		}
		backup, err = s.restoreWithReplace(ctx, source, latest.ID, func(staged, dest string) error {
			latest.Operation.State = "undo_pending"
			if err := writeSnapshotMetadata(ctx, root, latest); err != nil {
				return err
			}
			return replaceFile(staged, dest)
		}, latest.Operation.AfterSHA256)
		if err != nil {
			return err
		}
		latest.Operation.State = "undone"
		_ = writeSnapshotMetadata(ctx, root, latest)
		entry = HistoryEntry{*latest.Operation, latest.ID, latest.ExpiresAt}
		return nil
	})
	return entry, backup, err
}

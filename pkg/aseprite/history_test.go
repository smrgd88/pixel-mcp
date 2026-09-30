package aseprite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func historyEditBytes(ctx context.Context, p string, b []byte) error {
	staged, ok, err := boundSprite(ctx, p)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no staged binding")
	}
	return os.WriteFile(staged, b, 0600)
}
func TestHistoryUndoChainAndRetryGuard(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	require.NoError(t, s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("second")) }))
	require.NoError(t, s.WithOperation(ctx, p, "flatten_layers", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("third")) }))
	s = NewSnapshotStore(s.Dir)
	entries, err := s.ListHistory(ctx, p)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "flatten_layers", entries[0].Tool)
	undone, backup, err := s.UndoLast(ctx, p, entries[0].ID)
	require.NoError(t, err)
	require.Equal(t, "undone", undone.State)
	require.NotEmpty(t, backup.ID)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "second", string(b))
	_, _, err = s.UndoLast(ctx, p, entries[0].ID)
	require.ErrorContains(t, err, "history_operation_mismatch")
	_, _, err = s.UndoLast(ctx, p, entries[1].ID)
	require.NoError(t, err)
	b, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "original bytes", string(b))
	entries, err = s.ListHistory(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "undone", entries[0].State)
	require.Equal(t, "undone", entries[1].State)
}
func TestHistoryFailureNoopAndQuota(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	err := s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error {
		require.NoError(t, historyEditBytes(ctx, p, []byte("do not publish")))
		return errors.New("handler failed")
	})
	require.Error(t, err)
	require.NoError(t, s.WithOperation(ctx, p, "draw_pixels", func(context.Context) error { return nil }))
	entries, err := s.ListHistory(ctx, p)
	require.NoError(t, err)
	require.Empty(t, entries)
	s.MaxCount = 1
	require.NoError(t, s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("second")) }))
	err = s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("blocked")) })
	require.ErrorContains(t, err, "snapshot_capacity")
	entries, err = s.ListHistory(ctx, p)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	_, _, err = s.UndoLast(ctx, p, entries[0].ID)
	require.ErrorContains(t, err, "snapshot_capacity")
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "second", string(b))
}
func TestHistoryRejectsExternalChangesAndMissingRecovery(t *testing.T) {
	for _, kind := range []string{"external", "deleted", "tampered", "stale_id"} {
		t.Run(kind, func(t *testing.T) {
			s, p := snapshotFixture(t)
			ctx := context.Background()
			require.NoError(t, s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("edited")) }))
			entries, err := s.ListHistory(ctx, p)
			require.NoError(t, err)
			entry := entries[0]
			expected := "edited"
			id := entry.ID
			switch kind {
			case "external":
				expected = "external"
				require.NoError(t, os.WriteFile(p, []byte(expected), 0600))
			case "deleted":
				require.NoError(t, s.Delete(ctx, entry.SnapshotID))
			case "tampered":
				require.NoError(t, os.WriteFile(filepath.Join(s.Dir, entry.SnapshotID, "sprite"), []byte("corrupt!!bytes!"), 0600))
			case "stale_id":
				id = entry.SnapshotID
			}
			_, _, err = s.UndoLast(ctx, p, id)
			require.Error(t, err)
			b, err := os.ReadFile(p)
			require.NoError(t, err)
			require.Equal(t, expected, string(b))
		})
	}
}
func TestHistoryPendingReconciliation(t *testing.T) {
	for _, state := range []string{"pending", "undo_pending"} {
		for _, endpoint := range []string{"before", "after", "external"} {
			t.Run(state+"_"+endpoint, func(t *testing.T) {
				s, p := snapshotFixture(t)
				ctx := context.Background()
				require.NoError(t, s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("edited")) }))
				items, err := s.List(ctx, p)
				require.NoError(t, err)
				m := items[0]
				m.Operation.State = state
				root, err := s.root()
				require.NoError(t, err)
				require.NoError(t, writeSnapshotMetadata(ctx, root, m))
				if endpoint == "before" {
					require.NoError(t, os.WriteFile(p, []byte("original bytes"), 0600))
				}
				if endpoint == "external" {
					require.NoError(t, os.WriteFile(p, []byte("external"), 0600))
				}
				before, err := os.ReadFile(p)
				require.NoError(t, err)
				entries, err := s.ListHistory(ctx, p)
				if endpoint == "external" {
					require.ErrorContains(t, err, "history_recovery_required")
				} else {
					require.NoError(t, err)
					if state == "pending" && endpoint == "before" {
						require.Empty(t, entries)
					} else {
						require.Len(t, entries, 1)
						want := "applied"
						if endpoint == "before" {
							want = "undone"
						}
						require.Equal(t, want, entries[0].State)
					}
				}
				after, err := os.ReadFile(p)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}
func TestHistorySerializationAndRedaction(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	done := make(chan error, 2)
	for _, value := range []string{"first", "second"} {
		go func(v string) {
			done <- s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte(v)) })
		}(value)
	}
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	entries, err := s.ListHistory(ctx, p)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Greater(t, entries[0].Sequence, entries[1].Sequence)
	_, _, err = s.UndoLast(ctx, p, entries[0].ID)
	require.NoError(t, err)
	_, _, err = s.UndoLast(ctx, p, entries[1].ID)
	require.NoError(t, err)
}

func TestHistoryRestoreRechecksExpectedStateInsideStaging(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx := context.Background()
	require.NoError(t, s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("edited")) }))
	entries, err := s.ListHistory(ctx, p)
	require.NoError(t, err)
	entry := entries[0]
	// Represents an outside editor writing after UndoLast's preliminary check
	// but before restore's staging transaction captures the source.
	require.NoError(t, os.WriteFile(p, []byte("external edit"), 0600))
	_, err = s.restoreWithReplace(ctx, p, entry.SnapshotID, replaceFile, entry.AfterSHA256)
	require.ErrorContains(t, err, "history_conflict")
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "external edit", string(b))
	snapshots, err := s.List(ctx, p)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
}

func TestHistoryCancellationAndExpiry(t *testing.T) {
	s, p := snapshotFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	err := s.WithOperation(ctx, p, "draw_pixels", func(ctx context.Context) error {
		require.NoError(t, historyEditBytes(ctx, p, []byte("cancelled edit")))
		cancel()
		return nil
	})
	require.ErrorIs(t, err, context.Canceled)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "original bytes", string(b))
	entries, err := s.ListHistory(context.Background(), p)
	require.NoError(t, err)
	require.Empty(t, entries)
	s.TTL = time.Millisecond
	require.NoError(t, s.WithOperation(context.Background(), p, "draw_pixels", func(ctx context.Context) error { return historyEditBytes(ctx, p, []byte("edited")) }))
	time.Sleep(3 * time.Millisecond)
	entries, err = s.ListHistory(context.Background(), p)
	require.NoError(t, err)
	require.Empty(t, entries)
	b, err = os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "edited", string(b))
}

func TestHistoryCancelledLockWaitPreservesFile(t *testing.T) {
	s, p := snapshotFixture(t)
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- WithFileLocks(context.Background(), []string{p}, func(context.Context) error { close(locked); <-release; return nil })
	}()
	<-locked
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := s.WithOperation(ctx, p, "draw_pixels", func(context.Context) error { t.Error("handler ran while lock held"); return nil })
	close(release)
	require.NoError(t, <-done)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	b, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "original bytes", string(b))
}

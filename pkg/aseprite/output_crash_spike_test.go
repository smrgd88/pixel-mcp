//go:build linux || darwin

package aseprite

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This SPIKE exercises the real publisher, not an implemented recovery service.
// The child pauses at deterministic boundaries and the parent sends SIGKILL.
func TestOutputCrashSpikeChild(t *testing.T) {
	root := os.Getenv("PIXEL_CRASH_SPIKE_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	phase := os.Getenv("PIXEL_CRASH_SPIKE_PHASE")
	pause := func(at string) {
		if phase != at {
			return
		}
		fmt.Println("CRASH_SPIKE_READY")
		_, _ = os.Stdin.Read(make([]byte, 1))
		os.Exit(71) // EOF is not a valid injection; the parent must kill us.
	}
	paths := []string{filepath.Join(root, "sheet.png"), filepath.Join(root, "sheet.json")}
	published := 0
	err := withOutputFilesReplace(context.Background(), paths, func(staged []string) error {
		if err := writeOutputSet(staged); err != nil {
			return err
		}
		pause("staged")
		return nil
	}, func(src, dst string) error {
		pause("before-first")
		if err := replaceFile(src, dst); err != nil {
			return err
		}
		published++
		// Pause before returning: the real publisher has not yet set committed.
		pause("after-" + strconv.Itoa(published))
		return nil
	})
	require.NoError(t, err)
	t.Fatal("child never reached its crash checkpoint")
}

func TestOutputCrashSpikePublicationBoundaries(t *testing.T) {
	for _, existed := range []bool{false, true} {
		for _, tc := range []struct {
			phase string
			count int
		}{{"staged", 0}, {"before-first", 0}, {"after-1", 1}, {"after-2", 2}} {
			t.Run(fmt.Sprintf("existing=%t/%s", existed, tc.phase), func(t *testing.T) {
				root := outputTestDir(t)
				paths := []string{filepath.Join(root, "sheet.png"), filepath.Join(root, "sheet.json")}
				for i, p := range paths {
					if existed {
						require.NoError(t, os.WriteFile(p, []byte(fmt.Sprintf("old-%d", i)), 0640))
					}
				}
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOutputCrashSpikeChild$", "-test.v")
				cmd.Env = append(os.Environ(), "PIXEL_CRASH_SPIKE_ROOT="+root, "PIXEL_CRASH_SPIKE_PHASE="+tc.phase)
				stdout, err := cmd.StdoutPipe()
				require.NoError(t, err)
				input, err := cmd.StdinPipe()
				require.NoError(t, err)
				defer input.Close()
				require.NoError(t, cmd.Start())
				defer cmd.Process.Kill()
				scanner := bufio.NewScanner(stdout)
				ready := false
				for scanner.Scan() {
					if scanner.Text() == "CRASH_SPIKE_READY" {
						ready = true
						break
					}
				}
				killErr := cmd.Process.Kill()
				waitErr := cmd.Wait()
				require.True(t, ready, "child did not reach checkpoint: %v", waitErr)
				require.NoError(t, killErr)
				require.Error(t, waitErr)
				require.NoError(t, ctx.Err(), "deadline termination is not a checkpoint kill")
				for i, p := range paths {
					if i < tc.count {
						requireFileBytes(t, p, fmt.Sprintf("new-%d", i))
					} else if existed {
						requireFileBytes(t, p, fmt.Sprintf("old-%d", i))
					} else {
						_, err := os.Lstat(p)
						require.True(t, os.IsNotExist(err))
					}
				}
				stages, err := filepath.Glob(filepath.Join(root, ".pixel-mcp-stage-*"))
				require.NoError(t, err)
				require.Len(t, stages, 2, "SIGKILL bypasses deferred cleanup")
				backups, err := filepath.Glob(filepath.Join(root, ".pixel-mcp-stage-*", ".original-backup"))
				require.NoError(t, err)
				if existed {
					require.Len(t, backups, 2)
					old := map[string]bool{}
					for _, p := range backups {
						b, err := os.ReadFile(p)
						require.NoError(t, err)
						old[string(b)] = true
					}
					require.Equal(t, map[string]bool{"old-0": true, "old-1": true}, old)
				} else {
					require.Empty(t, backups)
				}
				lockCtx, stop := context.WithTimeout(context.Background(), time.Second)
				defer stop()
				require.NoError(t, WithFileLocks(lockCtx, paths, func(context.Context) error { return nil }))
				t.Logf("checkpoint=%s existing=%t published=%d stages=%d backups=%d lock=reacquired", tc.phase, existed, tc.count, len(stages), len(backups))
			})
		}
	}
}

// A small, pure decision model for a future trusted journal. No recovery API or
// filesystem mutation is introduced by these tests. Identity/path/authentication,
// durability and real rename races require implementation-phase tests.
type crashRecoveryObservation struct {
	existed, present, backupValid bool
	before, after, current        [32]byte
}

func modelCrashRollback(entries []crashRecoveryObservation, committed bool) ([]string, error) {
	actions := make([]string, len(entries))
	for i, e := range entries {
		switch {
		case committed:
			if !e.present || e.current != e.after {
				return nil, fmt.Errorf("committed generation changed at %d", i)
			}
			actions[i] = "keep-new"
		case e.existed && e.present && e.current == e.before:
			actions[i] = "keep-old"
		case !e.existed && !e.present:
			actions[i] = "keep-absent"
		case e.present && e.current == e.after && e.existed && e.backupValid:
			actions[i] = "restore-copy"
		case e.present && e.current == e.after && !e.existed:
			actions[i] = "remove-new"
		default:
			return nil, fmt.Errorf("conflict or missing backup at %d", i)
		}
	}
	return actions, nil
}

func TestOutputCrashSpikeRecoveryDecisionModel(t *testing.T) {
	before, after, external := sha256.Sum256([]byte("old")), sha256.Sum256([]byte("new")), sha256.Sum256([]byte("external"))
	old := crashRecoveryObservation{existed: true, present: true, backupValid: true, before: before, after: after, current: before}
	newFile := old
	newFile.current = after
	created := crashRecoveryObservation{present: true, after: after, current: after}
	absent := created
	absent.present = false
	for _, tc := range []struct {
		name      string
		entries   []crashRecoveryObservation
		committed bool
		want      []string
	}{
		{"prepared", []crashRecoveryObservation{old, old}, false, []string{"keep-old", "keep-old"}},
		{"rename-before-ack", []crashRecoveryObservation{newFile, old}, false, []string{"restore-copy", "keep-old"}},
		{"all-renamed-before-commit", []crashRecoveryObservation{newFile, newFile}, false, []string{"restore-copy", "restore-copy"}},
		{"rollback-after-one-restore", []crashRecoveryObservation{old, newFile}, false, []string{"keep-old", "restore-copy"}},
		{"rollback-complete-retry", []crashRecoveryObservation{old, absent}, false, []string{"keep-old", "keep-absent"}},
		{"created-output", []crashRecoveryObservation{created, old}, false, []string{"remove-new", "keep-old"}},
		{"commit-before-response", []crashRecoveryObservation{newFile, created}, true, []string{"keep-new", "keep-new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actions, err := modelCrashRollback(tc.entries, tc.committed)
			require.NoError(t, err)
			require.Equal(t, tc.want, actions)
			// Replay without using an in-memory committed bitmap.
			state := append([]crashRecoveryObservation(nil), tc.entries...)
			for i, action := range actions {
				if action == "restore-copy" {
					state[i].current = state[i].before
				}
				if action == "remove-new" {
					state[i].present = false
				}
			}
			again, err := modelCrashRollback(state, tc.committed)
			require.NoError(t, err)
			for _, action := range again {
				require.Contains(t, []string{"keep-old", "keep-absent", "keep-new"}, action)
			}
		})
	}
	for _, committed := range []bool{false, true} {
		changed := newFile
		changed.current = external
		actions, err := modelCrashRollback([]crashRecoveryObservation{newFile, changed}, committed)
		require.Error(t, err)
		require.Nil(t, actions, "preflight conflict must not return a partial execution plan")
	}
	missingBackup := newFile
	missingBackup.backupValid = false
	_, err := modelCrashRollback([]crashRecoveryObservation{missingBackup}, false)
	require.Error(t, err)
	missingOld := old
	missingOld.present = false
	_, err = modelCrashRollback([]crashRecoveryObservation{missingOld}, false)
	require.Error(t, err)
}

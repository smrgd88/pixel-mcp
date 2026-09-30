package aseprite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Snapshot is an immutable byte copy of a saved sprite, not Aseprite's undo state.
type Snapshot struct {
	Operation  *OperationRecord `json:"operation,omitempty"`
	ID         string           `json:"snapshot_id"`
	SpritePath string           `json:"sprite_path"`
	CreatedAt  time.Time        `json:"created_at"`
	ExpiresAt  time.Time        `json:"expires_at"`
	Size       int64            `json:"size_bytes"`
	SHA256     string           `json:"sha256"`
	Label      string           `json:"label,omitempty"`
}

// SnapshotStore serializes all store operations across processes. Limits apply
// to unexpired snapshots; capacity exhaustion never evicts a live recovery copy.
type SnapshotStore struct {
	Dir      string
	MaxCount int
	MaxBytes int64
	TTL      time.Duration
}

// NewSnapshotStore uses a 100-entry, 512 MiB, seven-day retention policy.
func NewSnapshotStore(dir string) *SnapshotStore {
	return &SnapshotStore{Dir: dir, MaxCount: 100, MaxBytes: 512 << 20, TTL: 7 * 24 * time.Hour}
}

func privateSnapshotPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("snapshot_invalid: non-regular storage path")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("snapshot_invalid: storage must be private")
	}
	if !directory {
		return checkSingleLink(path, info)
	}
	return nil
}

func (s *SnapshotStore) root() (string, error) {
	if s.MaxCount <= 0 || s.MaxBytes <= 0 || s.TTL <= 0 {
		return "", fmt.Errorf("snapshot_invalid: invalid limits")
	}
	dir := s.Dir
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "pixel-mcp", "snapshots")
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("snapshot_invalid: snapshot directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	if err := privateSnapshotPath(dir, true); err != nil {
		return "", err
	}
	return CanonicalPath(dir)
}

func validSnapshotID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.Version() == 4 && u.String() == id
}

// run locks the store and optional sprite together to avoid lock-order inversions.
func (s *SnapshotStore) run(ctx context.Context, source string, fn func(context.Context, string, string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := s.root()
	if err != nil {
		return err
	}
	paths := []string{root}
	if source != "" {
		paths = append(paths, source)
	}
	return WithFileLocks(ctx, paths, func(ctx context.Context) error {
		if err := privateSnapshotPath(root, true); err != nil {
			return err
		}
		original := ""
		if source != "" {
			original, err = CanonicalPath(source)
			if err != nil {
				return err
			}
			if !scopeOf(ctx).locked[pathKey(original)] {
				return fmt.Errorf("file_changed: snapshot source target changed")
			}
			rel, e := filepath.Rel(root, original)
			if e != nil {
				return e
			}
			if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
				return fmt.Errorf("snapshot_invalid: source must be outside snapshot storage")
			}
		}
		return fn(ctx, root, original)
	})
}

func readSnapshot(root, id string) (Snapshot, error) {
	var m Snapshot
	if !validSnapshotID(id) {
		return m, fmt.Errorf("snapshot_invalid: expected canonical UUID v4")
	}
	dir := filepath.Join(root, id)
	if err := privateSnapshotPath(dir, true); err != nil {
		return m, err
	}
	path := filepath.Join(dir, "metadata.json")
	if err := privateSnapshotPath(path, false); err != nil {
		return m, err
	}
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil {
		return m, err
	}
	if len(b) > 16384 {
		return m, fmt.Errorf("snapshot_invalid: metadata too large")
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("snapshot_invalid: metadata: %w", err)
	}
	hash, e := hex.DecodeString(m.SHA256)
	if !validOperation(m.Operation) || (m.Operation != nil && m.Operation.BeforeSHA256 != m.SHA256) || m.ID != id || !filepath.IsAbs(m.SpritePath) || m.Size <= 0 || e != nil || len(hash) != 32 || m.CreatedAt.IsZero() || !m.ExpiresAt.After(m.CreatedAt) {
		return m, fmt.Errorf("snapshot_invalid: invalid metadata")
	}
	path = filepath.Join(dir, "sprite")
	if err := privateSnapshotPath(path, false); err != nil {
		return m, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return m, err
	}
	if info.Size() != m.Size {
		return m, fmt.Errorf("snapshot_invalid: size mismatch")
	}
	return m, nil
}

// inventory also performs lazy expiration and crash-orphan cleanup under the
// exclusive store lock. Unexpected files are rejected, never silently removed.
func (s *SnapshotStore) inventory(ctx context.Context, root string) ([]Snapshot, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	out := []Snapshot{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		id := entry.Name()
		if strings.HasPrefix(id, ".pending-") && validSnapshotID(strings.TrimPrefix(id, ".pending-")) {
			if err := privateSnapshotPath(filepath.Join(root, id), true); err != nil {
				return nil, err
			}
			if err := os.RemoveAll(filepath.Join(root, id)); err != nil {
				return nil, err
			}
			continue
		}
		m, err := readSnapshot(root, id)
		if err != nil {
			return nil, fmt.Errorf("snapshot_invalid: store entry %s: %w", id, err)
		}
		if !time.Now().Before(m.ExpiresAt) {
			if err := os.RemoveAll(filepath.Join(root, id)); err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

type snapshotReader struct {
	ctx context.Context
	r   io.Reader
}

func (r snapshotReader) Read(p []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	return r.r.Read(p)
}

// snapshotSource bounds hashing before reading and observes cancellation between reads.
func snapshotSource(ctx context.Context, path string, limit int64) (fileSnapshot, error) {
	f, info, err := openRegularFile(path)
	if err != nil {
		return fileSnapshot{}, err
	}
	defer f.Close()
	if info.Size() <= 0 || info.Size() > limit {
		return fileSnapshot{}, fmt.Errorf("snapshot_capacity: empty source or byte limit exceeded")
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(snapshotReader{ctx, f}, limit+1))
	if err != nil {
		return fileSnapshot{}, err
	}
	if n > limit {
		return fileSnapshot{}, fmt.Errorf("snapshot_capacity: source grew beyond byte limit")
	}
	var hash [32]byte
	copy(hash[:], h.Sum(nil))
	return fileSnapshot{info: info, hash: hash}, nil
}

func copySnapshotBytes(ctx context.Context, src, dst string, limit int64) (int64, string, error) {
	f, _, err := openRegularFile(src)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return 0, "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(snapshotReader{ctx, f}, limit+1))
	if err == nil && n > limit {
		err = fmt.Errorf("snapshot_capacity: byte limit exceeded")
	}
	if err == nil {
		err = out.Sync()
	}
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	return n, hex.EncodeToString(h.Sum(nil)), err
}

func (s *SnapshotStore) create(ctx context.Context, root, source, label string, existing []Snapshot) (Snapshot, error) {
	var m Snapshot
	if len(label) > 256 {
		return m, fmt.Errorf("snapshot_invalid: label exceeds 256 bytes")
	}
	used := int64(0)
	for _, item := range existing {
		used += item.Size
	}
	if len(existing) >= s.MaxCount || used >= s.MaxBytes {
		return m, fmt.Errorf("snapshot_capacity: delete snapshots or wait for expiry")
	}
	before, err := snapshotSource(ctx, source, s.MaxBytes-used)
	if err != nil {
		return m, err
	}
	if before.info.Size() <= 0 || before.info.Size() > s.MaxBytes-used {
		return m, fmt.Errorf("snapshot_capacity: empty source or byte limit exceeded")
	}
	id := uuid.NewString()
	dir := filepath.Join(root, ".pending-"+id)
	if err = os.Mkdir(dir, 0700); err != nil {
		return m, err
	}
	defer os.RemoveAll(dir)
	size, hash, err := copySnapshotBytes(ctx, source, filepath.Join(dir, "sprite"), s.MaxBytes-used)
	if err != nil {
		return m, err
	}
	after, err := snapshotSource(ctx, source, s.MaxBytes-used)
	if err != nil || !os.SameFile(before.info, after.info) || before.hash != after.hash || before.info.Mode() != after.info.Mode() || hash != hex.EncodeToString(before.hash[:]) {
		return m, fmt.Errorf("file_changed: source changed during snapshot")
	}
	now := time.Now().UTC()
	m = Snapshot{ID: id, SpritePath: source, CreatedAt: now, ExpiresAt: now.Add(s.TTL), Size: size, SHA256: hash, Label: label}
	data, err := json.Marshal(m)
	if err != nil {
		return m, err
	}
	if len(data) > 16384 {
		return m, fmt.Errorf("snapshot_invalid: metadata too large")
	}
	f, err := os.OpenFile(filepath.Join(dir, "metadata.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return m, err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return m, err
	}
	if err = ctx.Err(); err != nil {
		return m, err
	}
	err = os.Rename(dir, filepath.Join(root, id))
	return m, err
}

// Create saves the original bytes without modifying or opening it in Aseprite.
func (s *SnapshotStore) Create(ctx context.Context, path, label string) (Snapshot, error) {
	var out Snapshot
	if path == "" {
		return out, fmt.Errorf("snapshot_invalid: sprite_path required")
	}
	err := s.run(ctx, path, func(ctx context.Context, root, source string) error {
		entries, err := s.inventory(ctx, root)
		if err != nil {
			return err
		}
		out, err = s.create(ctx, root, source, label, entries)
		return err
	})
	return out, err
}

// List returns unexpired metadata, optionally scoped to a canonical sprite path.
func (s *SnapshotStore) List(ctx context.Context, path string) ([]Snapshot, error) {
	out := []Snapshot{}
	err := s.run(ctx, path, func(ctx context.Context, root, source string) error {
		entries, err := s.inventory(ctx, root)
		if err != nil {
			return err
		}
		for _, m := range entries {
			if source == "" || pathKey(m.SpritePath) == pathKey(source) {
				out = append(out, m)
			}
		}
		return nil
	})
	return out, err
}

// Delete only removes the explicitly named snapshot; it never edits a sprite.
func (s *SnapshotStore) Delete(ctx context.Context, id string) error {
	if !validSnapshotID(id) {
		return fmt.Errorf("snapshot_invalid: expected canonical UUID v4")
	}
	return s.run(ctx, "", func(ctx context.Context, root, _ string) error {
		dir := filepath.Join(root, id)
		if err := privateSnapshotPath(dir, true); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return os.RemoveAll(dir)
	})
}

// Restore copies the saved bytes into same-directory staging and publishes
// atomically. A durable pre-restore snapshot must succeed before replacement.
// On publication failure the backup remains discoverable through List.
func (s *SnapshotStore) Restore(ctx context.Context, path, id string) (Snapshot, error) {
	return s.restoreWithReplace(ctx, path, id, replaceFile)
}

func (s *SnapshotStore) restoreWithReplace(ctx context.Context, path, id string, replace func(string, string) error, expectedSourceHash ...string) (Snapshot, error) {
	var backup Snapshot
	if path == "" || !validSnapshotID(id) {
		return backup, fmt.Errorf("snapshot_invalid: sprite_path and canonical UUID v4 required")
	}
	err := s.run(ctx, path, func(ctx context.Context, root, source string) error {
		entries, err := s.inventory(ctx, root)
		if err != nil {
			return err
		}
		m, err := readSnapshot(root, id)
		if err != nil {
			return err
		}
		if !time.Now().Before(m.ExpiresAt) {
			return fmt.Errorf("snapshot_expired")
		}
		if pathKey(source) != pathKey(m.SpritePath) {
			return fmt.Errorf("snapshot_source_mismatch: restore only to original sprite_path")
		}
		return stageFileWithReplace(ctx, source, true, func(staged string) error {
			// Undo's earlier lookup is not the transaction boundary: an outside
			// editor may have changed the file before stageFile captured it.
			if len(expectedSourceHash) > 0 {
				current, err := snapshotSource(ctx, source, s.MaxBytes)
				if err != nil {
					return err
				}
				if hex.EncodeToString(current.hash[:]) != expectedSourceHash[0] {
					return fmt.Errorf("history_conflict: file changed before restore staging")
				}
			}

			// stageFile seeded a working copy; remove only that private staged copy.
			if err := os.Remove(staged); err != nil {
				return err
			}
			n, hash, err := copySnapshotBytes(ctx, filepath.Join(root, id, "sprite"), staged, s.MaxBytes)
			if err != nil {
				return err
			}
			if n != m.Size || hash != m.SHA256 {
				return fmt.Errorf("snapshot_invalid: checksum mismatch")
			}
			backup, err = s.create(ctx, root, source, "before restore "+id, entries)
			return err
		}, replace)
	})
	return backup, err
}

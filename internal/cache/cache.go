// Package cache is a content-addressed build cache (shield-platform.md section
// 15; issue #12). Because the engine is deterministic (P2 — same input + policy
// + engine version => identical output), a build can be keyed by the hash of its
// inputs and reused instead of recomputed. This avoids reprocessing unchanged
// projects/libraries across builds.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Store is a directory of cache entries keyed by content hash.
type Store struct{ dir string }

// New opens (creating if needed) a cache rooted at dir.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) entry(key string) string { return filepath.Join(s.dir, key) }

// Get returns the cached entry directory for key if present. The protected
// output tree lives at <entry>/out and the evidence at <entry>/report.json.
func (s *Store) Get(key string) (string, bool) {
	e := s.entry(key)
	if fi, err := os.Stat(filepath.Join(e, "out")); err == nil && fi.IsDir() {
		return e, true
	}
	return "", false
}

// Report reads the cached report.json for key.
func (s *Store) Report(key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(s.entry(key), "report.json"))
}

// OutDir returns the cached output tree path for key.
func (s *Store) OutDir(key string) string { return filepath.Join(s.entry(key), "out") }

// Put stores outDir (the protected tree) and report under key, atomically (via a
// temp dir + rename), so a crashed/partial write is never observed as a hit.
func (s *Store) Put(key, outDir string, report []byte) error {
	e := s.entry(key)
	tmp := e + ".tmp"
	_ = os.RemoveAll(tmp)
	if err := CopyTree(outDir, filepath.Join(tmp, "out")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "report.json"), report, 0o644); err != nil {
		return err
	}
	_ = os.RemoveAll(e)
	return os.Rename(tmp, e)
}

// Key derives the cache key from the project files under root, the policy bytes
// and the engine version. Deterministic and order-independent.
func Key(root string, policyJSON []byte, version string) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "shield-cache|v=%s\n", version)
	h.Write(policyJSON)
	h.Write([]byte{'\n'})

	type fh struct{ rel, sum string }
	var files []fh
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum, err := hashFile(path)
		if err != nil {
			return err
		}
		files = append(files, fh{filepath.ToSlash(rel), sum})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%s\n", f.rel, f.sum)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CopyTree recursively copies src into dst.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}

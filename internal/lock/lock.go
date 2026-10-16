// Package lock reads and writes cali.lock, a deterministic JSON file: sorted
// keys, 2-space indent, LF line endings, trailing newline, no timestamps, no
// absolute paths, no per-machine data.
package lock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// currentVersion is the only lock format version this package understands.
const currentVersion = 1

// Entry pins one installed skill to an exact upstream commit and tree.
type Entry struct {
	Source string `json:"source"`
	Path   string `json:"path"`
	Ref    string `json:"ref"`
	Commit string `json:"commit"`
	Tree   string `json:"tree"`
}

// Lock is the in-memory form of cali.lock.
type Lock struct {
	Version int              `json:"version"`
	Skills  map[string]Entry `json:"skills"`
}

// Read loads the lock at path. A missing file yields an empty Lock with
// Version 1. Unknown fields, unknown versions and trailing data are errors.
// Errors name the path and the reason only, never file contents.
func Read(path string) (Lock, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Lock{Version: currentVersion, Skills: map[string]Entry{}}, nil
	}
	if err != nil {
		return Lock{}, fmt.Errorf("read lock %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var l Lock
	if err := dec.Decode(&l); err != nil {
		return Lock{}, fmt.Errorf("parse lock %s: malformed or contains unknown fields", path)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Lock{}, fmt.Errorf("parse lock %s: unexpected data after the JSON document", path)
	}
	if l.Version != currentVersion {
		return Lock{}, fmt.Errorf("parse lock %s: unsupported version %d (want %d)", path, l.Version, currentVersion)
	}
	if l.Skills == nil {
		l.Skills = map[string]Entry{}
	}
	return l, nil
}

// Marshal returns the canonical bytes of l. encoding/json sorts map keys, so
// the output depends only on the content.
func (l Lock) Marshal() []byte {
	if l.Skills == nil {
		l.Skills = map[string]Entry{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(l); err != nil {
		// Only strings, ints and a string-keyed map: cannot fail.
		panic("lock: marshal: " + err.Error())
	}
	return buf.Bytes() // Encode appends the single trailing "\n"
}

// Write atomically replaces the file at path: it writes a temp file in the
// same directory, then renames it over the target. The temp file is removed
// if any step fails.
func Write(path string, l Lock) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cali.lock-*.tmp")
	if err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close() // may already be closed; the error is irrelevant here
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(l.Marshal()); err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	// CreateTemp makes the file 0600; the lock is a committed, shared file.
	if err = os.Chmod(name, 0o644); err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("write lock %s: %w", path, err)
	}
	return nil
}

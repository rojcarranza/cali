// Package skillfs holds a skill folder as an in-memory tree and reads, writes,
// validates and compares it safely. Content comes from untrusted git repos, so
// nothing here follows symlinks or writes outside the target directory.
package skillfs

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
)

// Resource caps for a skill tree, enforced by Validate and ReadDir.
const (
	MaxFiles    = 10000
	MaxFileSize = 10 << 20
	MaxTreeSize = 100 << 20
)

// Package vars so tests can use small limits.
var (
	maxFiles          = MaxFiles
	maxFileSize int64 = MaxFileSize
	maxTreeSize int64 = MaxTreeSize
	compareExec       = runtime.GOOS != "windows"
)

// File is one regular file of a skill. Path is slash-separated and relative.
type File struct {
	Path string
	Exec bool
	Data []byte
}

// Tree is a set of files sorted by Path.
type Tree []File

// Change describes one path that differs between two trees.
type Change struct {
	Path string
	Kind string // "added", "removed" or "modified"
}

// ReadDir reads dir into a Tree without following symlinks. Any symlink or
// other non-regular entry is an error naming its relative path.
func ReadDir(dir string) (Tree, error) {
	var t Tree
	var total int64
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return fmt.Errorf("skill contains a symlink: %s", rel)
		case d.IsDir():
			return nil
		case !d.Type().IsRegular():
			return fmt.Errorf("skill contains a non-regular file: %s", rel)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if len(t) >= maxFiles {
			return fmt.Errorf("skill has more than %d files", maxFiles)
		}
		if info.Size() > maxFileSize {
			return fmt.Errorf("file %s exceeds the %d byte file size limit", rel, maxFileSize)
		}
		total += info.Size()
		if total > maxTreeSize {
			return fmt.Errorf("skill exceeds the %d byte total size limit at %s", maxTreeSize, rel)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		t = append(t, File{
			Path: rel,
			Exec: runtime.GOOS != "windows" && info.Mode().Perm()&0o111 != 0,
			Data: data,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(t, func(i, j int) bool { return t[i].Path < t[j].Path })
	return t, nil
}

// IsText applies the spec rule: no NUL byte in the first 8000 bytes.
func IsText(b []byte) bool {
	if len(b) > 8000 {
		b = b[:8000]
	}
	return bytes.IndexByte(b, 0) < 0
}

// Index maps each path of t to its file.
func Index(t Tree) map[string]*File {
	m := make(map[string]*File, len(t))
	for i := range t {
		m[t[i].Path] = &t[i]
	}
	return m
}

// FileEqualPtr is FileEqual where nil means absent: two absent files are equal.
func FileEqualPtr(a, b *File) bool {
	if a == nil || b == nil {
		return a == b
	}
	return FileEqual(*a, *b)
}

// FileEqual compares two files: Exec bits (except on windows), then text
// content after CRLF to LF, or binary content byte for byte.
func FileEqual(a, b File) bool {
	if compareExec && a.Exec != b.Exec {
		return false
	}
	if IsText(a.Data) && IsText(b.Data) {
		return bytes.Equal(NormalizeLF(a.Data), NormalizeLF(b.Data))
	}
	return bytes.Equal(a.Data, b.Data)
}

// NormalizeLF replaces every CRLF in b with LF.
func NormalizeLF(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func index(t Tree) map[string]File {
	m := make(map[string]File, len(t))
	for _, f := range t {
		m[f.Path] = f
	}
	return m
}

// Equal reports whether a and b have the same file set, Exec bits and
// contents. Text files are compared after CRLF to LF; binary files by bytes.
func Equal(a, b Tree) bool {
	return len(Diff(a, b)) == 0
}

// Diff lists the paths that differ going from base to other, sorted by path.
func Diff(base, other Tree) []Change {
	bm, om := index(base), index(other)
	var out []Change
	for p, bf := range bm {
		of, ok := om[p]
		switch {
		case !ok:
			out = append(out, Change{p, "removed"})
		case !FileEqual(bf, of):
			out = append(out, Change{p, "modified"})
		}
	}
	for p := range om {
		if _, ok := bm[p]; !ok {
			out = append(out, Change{p, "added"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// SameBytes compares trees byte for byte: same paths, exec bits and data.
// Unlike Equal it counts a line-ending-only change as a difference.
func SameBytes(a, b Tree) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Exec != b[i].Exec || !bytes.Equal(a[i].Data, b[i].Data) {
			return false
		}
	}
	return true
}

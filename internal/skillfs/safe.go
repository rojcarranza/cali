package skillfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	"CONIN$": true, "CONOUT$": true,
	"COM¹": true, "COM²": true, "COM³": true,
	"LPT¹": true, "LPT²": true, "LPT³": true,
}

// Validate rejects trees that must never be installed: empty trees, a missing
// root SKILL.md, duplicate paths, and any path that is absolute, contains
// "..", or has a segment Windows cannot hold. The checks apply on every OS so
// a skill authored on macOS cannot break a Windows install.
func Validate(t Tree) error {
	if len(t) == 0 {
		return errors.New("skill is empty")
	}
	if len(t) > maxFiles {
		return fmt.Errorf("skill has more than %d files", maxFiles)
	}
	type dirOwner struct{ prefix, path string }
	files := make(map[string]string, len(t)) // lower-cased path -> path
	dirs := make(map[string]dirOwner)        // lower-cased dir prefix -> first user
	hasSkill := false
	var total int64
	for _, f := range t {
		if err := validatePath(f.Path); err != nil {
			return err
		}
		if int64(len(f.Data)) > maxFileSize {
			return fmt.Errorf("file %s exceeds the %d byte file size limit", quote(f.Path), maxFileSize)
		}
		total += int64(len(f.Data))
		if total > maxTreeSize {
			return fmt.Errorf("skill exceeds the %d byte total size limit at %s", maxTreeSize, quote(f.Path))
		}
		lower := strings.ToLower(f.Path)
		if prev, ok := files[lower]; ok {
			if prev == f.Path {
				return fmt.Errorf("duplicate path: %s", quote(f.Path))
			}
			return fmt.Errorf("paths differ only by case: %s and %s", quote(prev), quote(f.Path))
		}
		files[lower] = f.Path
		for i := 0; i < len(f.Path); i++ {
			if f.Path[i] != '/' {
				continue
			}
			prefix := f.Path[:i]
			key := strings.ToLower(prefix)
			if o, ok := dirs[key]; ok && o.prefix != prefix {
				return fmt.Errorf("directories differ only by case: %s and %s", quote(o.path), quote(f.Path))
			} else if !ok {
				dirs[key] = dirOwner{prefix, f.Path}
			}
		}
		if f.Path == "SKILL.md" {
			hasSkill = true
		}
	}
	for key, p := range files {
		if o, ok := dirs[key]; ok {
			return fmt.Errorf("path %s is also a directory containing %s", quote(p), quote(o.path))
		}
	}
	if !hasSkill {
		return errors.New("skill has no SKILL.md at its root")
	}
	return nil
}

func validatePath(p string) error {
	if p == "" {
		return errors.New("empty path")
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || (len(p) >= 2 && p[1] == ':') {
		return fmt.Errorf("absolute or non-portable path: %s", quote(p))
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("path has an empty, . or .. segment: %s", quote(p))
		}
		if err := validateSegment(seg); err != nil {
			return fmt.Errorf("%w in path %s", err, quote(p))
		}
	}
	return nil
}

func validateSegment(seg string) error {
	for _, r := range seg {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`<>:"|?*`, r) {
			return errors.New("forbidden character")
		}
	}
	if last := seg[len(seg)-1]; last == '.' || last == ' ' {
		return errors.New("name ends in a dot or space")
	}
	stem := seg
	if i := strings.IndexByte(stem, '.'); i >= 0 {
		stem = stem[:i]
	}
	if strings.EqualFold(seg, ".git") {
		return errors.New("reserved name .git")
	}
	if reservedNames[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return errors.New("reserved Windows name")
	}
	return nil
}

// safePath is the part of validatePath that keeps a write inside its dir.
func safePath(p string) error {
	if p == "" {
		return errors.New("empty path")
	}
	if strings.HasPrefix(p, "/") || filepath.IsAbs(filepath.FromSlash(p)) || filepath.VolumeName(filepath.FromSlash(p)) != "" {
		return fmt.Errorf("absolute path: %s", quote(p))
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("path has a .. segment: %s", quote(p))
		}
	}
	return nil
}

func quote(s string) string { return fmt.Sprintf("%q", s) }

// WriteTo validates t and writes it under dir. Nothing is written if
// validation fails. dir must not exist or must be an empty real directory;
// files are created exclusively and no symlink is followed. It is not atomic:
// callers are responsible for writing into a staging directory and swapping
// it into place.
func (t Tree) WriteTo(dir string) error {
	if err := Validate(t); err != nil {
		return err
	}
	return t.WriteUnchecked(dir)
}

// WriteUnchecked writes a user's own tree (a backup, a patch side) under dir
// like WriteTo, but skips the upstream content rules (root SKILL.md, reserved
// names, .git, case collisions, caps): a local folder may break them and must
// still be saved. Path safety stays: no empty, absolute or .. path, nothing
// outside dir, no symlink followed, files created exclusively.
func (t Tree) WriteUnchecked(dir string) error {
	for _, f := range t {
		if err := safePath(f.Path); err != nil {
			return err
		}
	}
	root := filepath.Clean(dir)
	fi, err := os.Lstat(root)
	switch {
	case os.IsNotExist(err):
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
	case err != nil:
		return err
	case fi.Mode()&os.ModeSymlink != 0:
		return fmt.Errorf("refusing to write into symlinked directory: %s", dir)
	case !fi.IsDir():
		return fmt.Errorf("target is not a directory: %s", dir)
	default:
		entries, err := os.ReadDir(root)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("target directory is not empty: %s", dir)
		}
	}
	for _, f := range t {
		target := filepath.Join(root, filepath.FromSlash(f.Path))
		rel, err := filepath.Rel(root, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("path escapes target directory: %s", quote(f.Path))
		}
		if err := ensureDirs(root, filepath.Dir(target)); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Exec && runtime.GOOS != "windows" {
			mode = 0o755
		}
		if err := writeNew(target, f.Data, mode); err != nil {
			return fmt.Errorf("write %s: %w", quote(f.Path), err)
		}
	}
	return nil
}

// writeNew creates p exclusively (never follows or replaces an existing
// entry) and sets its mode through the open file.
func writeNew(p string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := f.Chmod(mode); err != nil {
			_ = f.Close()
			return err
		}
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ensureDirs creates each directory from root down to dir, refusing any
// existing component that is a symlink or not a directory.
func ensureDirs(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	cur := root
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		switch {
		case os.IsNotExist(err):
			if err := os.Mkdir(cur, 0o755); err != nil {
				return err
			}
		case err != nil:
			return err
		case fi.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("refusing to write through symlink: %s", quote(filepath.ToSlash(strings.TrimPrefix(cur, root+string(filepath.Separator)))))
		case !fi.IsDir():
			return fmt.Errorf("path component is not a directory: %s", quote(filepath.ToSlash(strings.TrimPrefix(cur, root+string(filepath.Separator)))))
		}
	}
	return nil
}

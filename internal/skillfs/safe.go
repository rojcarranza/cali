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
}

// Validate rejects trees that must never be installed: empty trees, a missing
// root SKILL.md, duplicate paths, and any path that is absolute, contains
// "..", or has a segment Windows cannot hold. The checks apply on every OS so
// a skill authored on macOS cannot break a Windows install.
func Validate(t Tree) error {
	if len(t) == 0 {
		return errors.New("skill is empty")
	}
	seen := make(map[string]bool, len(t))
	hasSkill := false
	for _, f := range t {
		if err := validatePath(f.Path); err != nil {
			return err
		}
		if seen[f.Path] {
			return fmt.Errorf("duplicate path: %s", f.Path)
		}
		seen[f.Path] = true
		if f.Path == "SKILL.md" {
			hasSkill = true
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
	if reservedNames[strings.ToUpper(strings.TrimRight(stem, " "))] {
		return errors.New("reserved Windows name")
	}
	return nil
}

func quote(s string) string { return fmt.Sprintf("%q", s) }

// WriteTo validates t and writes it under dir. Nothing is written if
// validation fails. It refuses to write through any symlink inside dir.
func (t Tree) WriteTo(dir string) error {
	if err := Validate(t); err != nil {
		return err
	}
	root := filepath.Clean(dir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
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
		if fi, err := os.Lstat(target); err == nil {
			if !fi.Mode().IsRegular() {
				return fmt.Errorf("refusing to overwrite non-regular file: %s", quote(f.Path))
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		mode := os.FileMode(0o644)
		if f.Exec && runtime.GOOS != "windows" {
			mode = 0o755
		}
		if err := os.WriteFile(target, f.Data, mode); err != nil {
			return err
		}
		if err := os.Chmod(target, mode); err != nil {
			return err
		}
	}
	return nil
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

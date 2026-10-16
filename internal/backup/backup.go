// Package backup keeps copies of skill folders that Cali is about to remove
// or overwrite, under <data>/backups/<name>/<UTC timestamp>/.
package backup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
)

// now is replaced in tests.
var now = time.Now

// Save writes t to <dataDir>/backups/<name>/<20060102T150405Z>/files/ and,
// when patch is non-nil, patch to local.patch beside files/. A timestamp dir
// that exists already gets a -1, -2, ... suffix. It returns the backup dir.
func Save(dataDir, name string, t skillfs.Tree, patch []byte) (string, error) {
	if !manifest.SkillNameValid(name) {
		return "", fmt.Errorf("invalid skill name %q for a backup", name)
	}
	parent := filepath.Join(dataDir, "backups", name)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	base := filepath.Join(parent, now().UTC().Format("20060102T150405Z"))
	dir := base
	// Mkdir claims the dir atomically, so two saves never share one.
	for i := 1; ; i++ {
		err := os.Mkdir(dir, 0o755)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		dir = fmt.Sprintf("%s-%d", base, i)
	}
	if err := t.WriteTo(filepath.Join(dir, "files")); err != nil {
		return "", fmt.Errorf("write backup %s: %w", dir, err)
	}
	if patch != nil {
		if err := os.WriteFile(filepath.Join(dir, "local.patch"), patch, 0o644); err != nil {
			return "", fmt.Errorf("write backup %s: %w", dir, err)
		}
	}
	return dir, nil
}

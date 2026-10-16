package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rojcarranza/cali/internal/backup"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// Remove backs up and deletes the installed folders of skill name in the
// scope d.Global picks, then drops its manifest and lock entries. A folder
// that differs from its pristine copy (or has none) blocks the remove unless
// force is set; the check runs before anything is changed. Pristine copies
// are never deleted.
func Remove(_ context.Context, d Deps, name string, force bool) error {
	opts := source.Options{AllowFile: d.AllowFileScheme}
	out := d.Stdout
	if out == nil {
		out = io.Discard
	}
	sc, err := resolveScope(d, d.Global)
	if err != nil {
		return err
	}
	if err := checkScopeFiles(sc); err != nil {
		return err
	}
	m, err := manifest.ReadWith(sc.manifest, opts)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("skill %q is not in %s", name, sc.manifest)
	}
	if err != nil {
		return err
	}
	if _, ok := m.Skills[name]; !ok {
		return fmt.Errorf("skill %q is not in %s", name, sc.manifest)
	}
	l, err := lock.Read(sc.lockPath)
	if err != nil {
		return err
	}
	ent, locked := l.Skills[name]
	var base skillfs.Tree
	haveBase := false
	if locked {
		if base, haveBase, err = store.Get(d.Dirs.Data, ent.Tree); err != nil {
			return err
		}
	}

	type folder struct {
		target, final string
		t             skillfs.Tree
	}
	var found []folder
	for _, tg := range m.TargetsFor(name) {
		dir, err := targets.Dir(tg, sc.scope, sc.root, d.Dirs.Home)
		if err != nil {
			return err
		}
		if sc.scope == targets.Project {
			if err := noSymlinks(sc.root, dir); err != nil {
				return err
			}
		}
		final := filepath.Join(dir, name)
		fi, err := os.Lstat(final)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case err != nil:
			return err
		case fi.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%s is a symlink; Cali does not remove through symlinks", final)
		case !fi.IsDir():
			return fmt.Errorf("%s is not a directory; not removing it", final)
		}
		t, err := skillfs.ReadDir(final)
		if err != nil {
			return fmt.Errorf("cannot back up %s: %w", final, err)
		}
		if !force && (!haveBase || !skillfs.Equal(t, base)) {
			return fmt.Errorf("%s has local edits (or no pristine copy to compare); pass --force to remove it anyway (a backup is kept)", final)
		}
		found = append(found, folder{tg, final, t})
	}

	for _, f := range found {
		p, err := backup.Save(d.Dirs.Data, name, f.t, nil)
		if err != nil {
			return fmt.Errorf("back up %s: %w", f.final, err)
		}
		_, _ = fmt.Fprintf(out, "backed up %s (%s) to %s\n", name, f.target, p)
		if err := os.RemoveAll(f.final); err != nil {
			return fmt.Errorf("remove %s: %w", f.final, err)
		}
		_, _ = fmt.Fprintf(out, "removed %s (%s)\n", name, f.target)
	}

	delete(m.Skills, name)
	if err := manifest.WriteWith(sc.manifest, m, opts); err != nil {
		return err
	}
	if locked {
		delete(l.Skills, name)
		if err := lock.Write(sc.lockPath, l); err != nil {
			return err
		}
	}
	return nil
}

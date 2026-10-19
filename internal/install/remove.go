package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rojcarranza/cali/internal/backup"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// afterBackup, when set by a test, runs between the backup and the removal.
var afterBackup func(final string)

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
		if afterBackup != nil {
			afterBackup(f.final)
		}
	}

	// Move every folder aside and check it before deleting any, so a folder
	// that changed stops the remove with every target still in place.
	var aside []string
	restore := func(cause error) error {
		errs := []error{cause}
		for i, old := range aside {
			if err := os.Rename(old, found[i].final); err != nil {
				errs = append(errs, fmt.Errorf("could not move %s back, it is at %s: %w", found[i].final, old, err))
			}
		}
		return errors.Join(errs...)
	}
	for _, f := range found {
		old, err := MoveAside(f.final, os.Rename)
		if err != nil {
			return restore(err)
		}
		aside = append(aside, old)
		// The folder may have changed since it was checked; never delete bytes the backup lacks.
		if now, err := skillfs.ReadDir(old); err != nil || !skillfs.SameBytes(now, f.t) {
			return restore(fmt.Errorf("%s changed while it was being backed up; not removing it", f.final))
		}
	}
	for i, old := range aside {
		if err := os.RemoveAll(old); err != nil {
			cause := fmt.Errorf("remove %s (moved to %s): %w", found[i].final, old, err)
			// Put the folders not yet removed back, so they are not stranded at .cali-old-*.
			aside, found = aside[i+1:], found[i+1:]
			return restore(cause)
		}
		_, _ = fmt.Fprintf(out, "removed %s (%s)\n", name, found[i].target)
	}

	restoreCmd := []string{"cali", "add"}
	if d.Global {
		restoreCmd = append(restoreCmd, "-g")
	}
	if d.Manifest != "" {
		restoreCmd = append(restoreCmd, "--manifest", d.Manifest)
	}
	src := m.Skills[name].Source
	if r := m.Skills[name].Ref; r != "" { // a pin kept in ref: goes back on the spec
		if sp, err := source.ParseWith(src, opts); err == nil {
			sp.Ref = r
			src = sp.String()
		}
	}
	restoreCmd = append(restoreCmd, src)
	for _, tg := range m.TargetsFor(name) {
		restoreCmd = append(restoreCmd, "--target", tg)
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
	_, _ = fmt.Fprintf(out, "to restore: %s (installs upstream; your old copy is in the backup above)\n", strings.Join(restoreCmd, " "))
	return nil
}

package install

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// Action is one thing Sync did or reports.
type Action struct {
	Kind   string // installed, adopted, kept or base-lost
	Name   string
	Target string // empty for base-lost
	Detail string // for kept: modified or unmanaged
}

func (a Action) String() string {
	switch {
	case a.Target == "":
		return a.Kind + " " + a.Name
	case a.Detail != "":
		return fmt.Sprintf("%s %s (%s): %s", a.Kind, a.Name, a.Target, a.Detail)
	default:
		return fmt.Sprintf("%s %s (%s)", a.Kind, a.Name, a.Target)
	}
}

// Report is what Sync did.
type Report struct{ Actions []Action }

// Sync makes the scope's target folders match cali.lock (d.Global picks the
// scope). Manifest skills missing from the lock are resolved and locked; lock
// entries no longer in the manifest are dropped. With frozen, either case is
// an error and nothing is written. Sync never deletes or overwrites a folder:
// a folder that differs is reported and left alone.
func Sync(ctx context.Context, d Deps, frozen bool) (Report, error) {
	opts := source.Options{AllowFile: d.AllowFileScheme}
	out := d.Stdout
	if out == nil {
		out = io.Discard
	}
	sc, err := resolveScope(d, d.Global)
	if err != nil {
		return Report{}, err
	}
	if err := checkScopeFiles(sc); err != nil {
		return Report{}, err
	}
	m, err := manifest.ReadWith(sc.manifest, opts)
	if errors.Is(err, os.ErrNotExist) {
		m, err = manifest.Manifest{Version: 1, Skills: map[string]manifest.Skill{}}, nil
	}
	if err != nil {
		return Report{}, err
	}
	l, err := lock.Read(sc.lockPath)
	if err != nil {
		return Report{}, err
	}

	var names, missing, stale []string
	for n := range m.Skills {
		names = append(names, n)
		if _, ok := l.Skills[n]; !ok {
			missing = append(missing, n)
		}
	}
	for n := range l.Skills {
		if _, ok := m.Skills[n]; !ok {
			stale = append(stale, n)
		}
	}
	slices.Sort(names)
	slices.Sort(missing)
	slices.Sort(stale)
	if frozen && (len(missing) > 0 || len(stale) > 0) {
		var why []string
		if len(missing) > 0 {
			why = append(why, "not locked: "+strings.Join(missing, ", "))
		}
		if len(stale) > 0 {
			why = append(why, "no longer in the manifest: "+strings.Join(stale, ", "))
		}
		return Report{}, fmt.Errorf("%s needs changes (%s); run sync without --frozen", sc.lockPath, strings.Join(why, "; "))
	}

	newL := lock.Lock{Version: l.Version, Skills: map[string]lock.Entry{}}
	for n, e := range l.Skills {
		if _, ok := m.Skills[n]; ok {
			newL.Skills[n] = e
		}
	}
	repos := map[string]*gitx.Repo{}
	fetch := func(spec source.Spec) (*gitx.Repo, error) {
		if r, ok := repos[spec.URL]; ok {
			return r, nil
		}
		r, err := openFetch(ctx, d, spec)
		if err == nil {
			repos[spec.URL] = r
		}
		return r, err
	}

	var rep Report
	trees := map[string]skillfs.Tree{} // pristine tree per skill that can be checked
	for _, n := range names {
		if slices.Contains(missing, n) {
			s := m.Skills[n]
			spec, err := source.ParseWith(s.Source, opts)
			if err != nil {
				return Report{}, fmt.Errorf("skill %s: %w", n, err)
			}
			repo, err := fetch(spec)
			if err != nil {
				return Report{}, err
			}
			ref := s.Ref
			if ref == "" {
				ref = spec.Ref
			}
			commit, err := repo.Resolve(ref)
			if err != nil {
				return Report{}, fmt.Errorf("skill %s: %w", n, err)
			}
			p := strings.Trim(spec.Path, "/")
			tree, t, err := exportSkill(repo, commit, p, n)
			if err != nil {
				return Report{}, err
			}
			if err := store.Put(d.Dirs.Data, tree, t); err != nil {
				return Report{}, fmt.Errorf("store pristine copy of %s: %w", n, err)
			}
			newL.Skills[n] = lock.Entry{Source: spec.URL, Path: p, Ref: ref, Commit: commit, Tree: tree}
			trees[n] = t
			continue
		}
		e := l.Skills[n]
		t, ok, err := store.Get(d.Dirs.Data, e.Tree)
		if err != nil {
			return Report{}, fmt.Errorf("pristine copy of %s: %w", n, err)
		}
		if !ok {
			spec, err := source.ParseWith(e.Source, opts)
			if err != nil {
				return Report{}, fmt.Errorf("skill %s: lock source: %w", n, err)
			}
			repo, err := fetch(spec)
			if err != nil {
				return Report{}, err
			}
			if !repo.HasCommit(e.Commit) {
				rep.Actions = append(rep.Actions, Action{Kind: "base-lost", Name: n})
				continue
			}
			tree, et, err := exportSkill(repo, e.Commit, e.Path, n)
			if err != nil {
				return Report{}, err
			}
			if tree != e.Tree {
				return Report{}, fmt.Errorf("skill %s: tree at commit %s is %s, but the lock says %s", n, e.Commit, tree, e.Tree)
			}
			if err := store.Put(d.Dirs.Data, tree, et); err != nil {
				return Report{}, fmt.Errorf("store pristine copy of %s: %w", n, err)
			}
			t = et
		}
		trees[n] = t
	}

	// Check every target folder before writing any.
	type job struct {
		name, target, final string
		t                   skillfs.Tree
	}
	var jobs []job
	for _, n := range names {
		t, ok := trees[n]
		if !ok {
			continue // base-lost
		}
		fresh := slices.Contains(missing, n)
		tgs := m.TargetsFor(n)
		if len(tgs) == 0 {
			return Report{}, fmt.Errorf("skill %s: no targets: set targets in %s", n, sc.manifest)
		}
		for _, tg := range tgs {
			dir, err := targets.Dir(tg, sc.scope, sc.root, d.Dirs.Home)
			if err != nil {
				return Report{}, err
			}
			if sc.scope == targets.Project {
				if err := noSymlinks(sc.root, dir); err != nil {
					return Report{}, err
				}
			}
			final := filepath.Join(dir, n)
			fi, err := os.Lstat(final)
			if errors.Is(err, os.ErrNotExist) {
				jobs = append(jobs, job{n, tg, final, t})
				continue
			}
			if err != nil {
				return Report{}, err
			}
			same := false
			if fi.IsDir() { // Lstat: a symlink is never IsDir, so it counts as differing
				have, rerr := skillfs.ReadDir(final)
				same = rerr == nil && skillfs.Equal(have, t)
			}
			switch {
			case same && fresh:
				rep.Actions = append(rep.Actions, Action{Kind: "adopted", Name: n, Target: tg})
			case same:
			case fresh:
				// Same rule as add: an unmanaged folder that differs is never taken over.
				rep.Actions = append(rep.Actions, Action{Kind: "kept", Name: n, Target: tg, Detail: "unmanaged"})
				delete(newL.Skills, n)
			default:
				rep.Actions = append(rep.Actions, Action{Kind: "kept", Name: n, Target: tg, Detail: "modified"})
			}
		}
	}

	rename := d.Rename
	if rename == nil {
		rename = os.Rename
	}
	var made []string
	rollback := func() {
		for _, dir := range made {
			_ = os.RemoveAll(dir) // created by this Sync; best effort
		}
	}
	for _, j := range jobs {
		if err := place(j.final, j.t, rename); err != nil {
			rollback()
			return Report{}, err
		}
		made = append(made, j.final)
		rep.Actions = append(rep.Actions, Action{Kind: "installed", Name: j.name, Target: j.target})
	}

	if !bytes.Equal(newL.Marshal(), l.Marshal()) {
		if err := os.MkdirAll(filepath.Dir(sc.lockPath), 0o755); err != nil {
			rollback()
			return Report{}, err
		}
		if err := lock.Write(sc.lockPath, newL); err != nil {
			rollback()
			return Report{}, err
		}
	}
	for _, a := range rep.Actions {
		_, _ = fmt.Fprintln(out, a.String())
	}
	return rep, nil
}

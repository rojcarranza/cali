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
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// Action is one thing Sync did or reports.
type Action struct {
	Kind   string // installed, adopted, kept, base-lost, ref changed or skipped
	Name   string
	Target string // empty for base-lost and ref changed
	Detail string // for kept: modified or unmanaged; for ref changed: the hint; for skipped: the reason
}

func (a Action) String() string {
	switch {
	case a.Kind == "skipped":
		return a.Name + ": skipped: " + a.Detail
	case a.Target == "" && a.Detail != "":
		return a.Kind + " " + a.Name + ": " + a.Detail
	case a.Target == "":
		return a.Kind + " " + a.Name
	case a.Detail != "":
		return fmt.Sprintf("%s %s (%s): %s", a.Kind, a.Name, a.Target, a.Detail)
	default:
		return fmt.Sprintf("%s %s (%s)", a.Kind, a.Name, a.Target)
	}
}

// ErrSkipped is returned, after everything else was done, when a skill was
// skipped because its upstream folder cannot be installed (or, from update,
// because it is not locked).
var ErrSkipped = errors.New("one or more skills were skipped")

// Report is what Sync did.
type Report struct{ Actions []Action }

// Sync makes the scope's target folders match cali.lock (d.Global picks the
// scope). Manifest skills missing from the lock are resolved and locked; lock
// entries no longer in the manifest are dropped. With frozen, either case is
// an error and nothing is written. Sync never deletes or overwrites a folder:
// a folder that differs is reported and left alone. A skill whose upstream folder
// fails validation, or whose ref or folder is gone upstream before it is locked,
// is reported as skipped, with its lock entry and folders left as they were;
// the rest is done and Sync then returns ErrSkipped.
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
	noManifest := errors.Is(err, os.ErrNotExist)
	if noManifest {
		m, err = manifest.Manifest{Version: 1, Skills: map[string]manifest.Skill{}}, nil
	}
	if err != nil {
		return Report{}, err
	}
	l, err := lock.Read(sc.lockPath)
	if err != nil {
		return Report{}, err
	}
	if noManifest && len(l.Skills) > 0 {
		// Treating it as empty would drop every lock entry.
		return Report{}, fmt.Errorf("%s not found but %s has skills; restore the manifest first", sc.manifest, sc.lockPath)
	}

	// A lock entry counts only if it matches the manifest's source and path;
	// otherwise a committed lock could point sync at another origin. A ref
	// change alone is left to update, which keeps the merge base.
	var names, missing, stale, refChanged []string
	specs := map[string]source.Spec{}
	refs := map[string]string{}
	for n, s := range m.Skills {
		names = append(names, n)
		spec, err := source.ParseWith(s.Source, opts)
		if err != nil {
			return Report{}, fmt.Errorf("skill %s: %w", n, err)
		}
		ref := s.EffectiveRef(spec)
		specs[n], refs[n] = spec, ref
		e, ok := l.Skills[n]
		switch {
		case !ok || !source.SameURL(e.Source, spec.URL, opts) || e.Path != strings.Trim(spec.Path, "/"):
			missing = append(missing, n)
		case e.Ref != ref:
			refChanged = append(refChanged, n)
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
	// A same-repo lock entry falls behind when the skill changes in a commit
	// that does not also move the lock; it is reported, and update moves it.
	var behind []string
	for n, spec := range specs {
		if spec.URL != source.Here || slices.Contains(missing, n) || slices.Contains(refChanged, n) {
			continue
		}
		repo, err := fetch(spec)
		if err != nil {
			return Report{}, err
		}
		commit, err := repo.Resolve("")
		if err != nil {
			return Report{}, fmt.Errorf("skill %s: %w", n, err)
		}
		if tree, err := repo.TreeSHA(commit, l.Skills[n].Path); err != nil || tree != l.Skills[n].Tree {
			behind = append(behind, n) // a folder gone from HEAD is behind too
		}
	}
	for n := range l.Skills {
		if _, ok := m.Skills[n]; !ok {
			stale = append(stale, n)
		}
	}
	slices.Sort(names)
	slices.Sort(behind)
	slices.Sort(missing)
	slices.Sort(stale)
	slices.Sort(refChanged)
	if frozen && (len(missing) > 0 || len(stale) > 0 || len(refChanged) > 0 || len(behind) > 0) {
		var why []string
		if len(missing) > 0 {
			why = append(why, "not locked, or locked with another source or path: "+strings.Join(missing, ", "))
		}
		if len(refChanged) > 0 {
			why = append(why, "ref changed in the manifest, run cali update: "+strings.Join(refChanged, ", "))
		}
		if len(stale) > 0 {
			why = append(why, "no longer in the manifest: "+strings.Join(stale, ", "))
		}
		if len(behind) > 0 {
			why = append(why, "locked tree is behind the repo, run cali update: "+strings.Join(behind, ", "))
		}
		return Report{}, fmt.Errorf("%s needs changes (%s); run sync without --frozen", sc.lockPath, strings.Join(why, "; "))
	}

	newL := lock.Lock{Version: l.Version, Skills: map[string]lock.Entry{}}
	for n, e := range l.Skills {
		if _, ok := m.Skills[n]; ok && !slices.Contains(missing, n) {
			newL.Skills[n] = e
		}
	}

	var rep Report
	trees := map[string]skillfs.Tree{} // pristine tree per skill that can be checked
	// skip reports a skill whose upstream folder cannot be installed; its lock
	// entry and folders stay as they were.
	skip := func(n string, err error) {
		if e, ok := l.Skills[n]; ok {
			newL.Skills[n] = e
		}
		rep.Actions = append(rep.Actions, Action{Kind: "skipped", Name: n, Detail: render.Sanitize(err.Error())})
	}
	for _, n := range names {
		spec := specs[n]
		if slices.Contains(refChanged, n) {
			// Lock entry, pristine copy and folders stay as they are.
			rep.Actions = append(rep.Actions, Action{Kind: "ref changed", Name: n, Detail: "run cali update"})
			continue
		}
		if slices.Contains(missing, n) {
			repo, err := fetch(spec)
			if err != nil {
				return Report{}, err
			}
			ref := refs[n]
			commit, err := repo.Resolve(ref)
			if errors.Is(err, gitx.ErrNotFound) {
				skip(n, err)
				continue
			}
			if err != nil {
				return Report{}, fmt.Errorf("skill %s: %w", n, err)
			}
			p := strings.Trim(spec.Path, "/")
			tree, t, err := exportSkill(repo, commit, p, n)
			if errors.Is(err, skillfs.ErrInvalid) || errors.Is(err, gitx.ErrNotFound) {
				skip(n, err)
				continue
			}
			if err != nil {
				return Report{}, err
			}
			if err := store.Put(d.Dirs.Data, tree, t); err != nil {
				return Report{}, fmt.Errorf("store pristine copy of %s: %w", n, err)
			}
			newL.Skills[n] = lock.Entry{Source: spec.URL, Path: p, Ref: ref, Commit: LockCommit(spec.URL, commit), Tree: tree}
			trees[n] = t
			continue
		}
		if slices.Contains(behind, n) {
			rep.Actions = append(rep.Actions, Action{Kind: "outdated", Name: n, Detail: "run cali update"})
		}
		e := l.Skills[n]
		t, ok, err := store.Get(d.Dirs.Data, e.Tree)
		if err != nil {
			return Report{}, fmt.Errorf("pristine copy of %s: %w", n, err)
		}
		if !ok {
			repo, err := fetch(spec) // the manifest's origin, which e.Source was checked to equal
			if err != nil {
				return Report{}, err
			}
			tree := e.Tree
			if spec.URL == source.Here {
				// No commit to check against: the tree is read by its SHA,
				// which names its content.
				if !repo.HasTree(tree) {
					rep.Actions = append(rep.Actions, Action{Kind: "base-lost", Name: n})
					continue
				}
			} else {
				if !repo.HasCommit(e.Commit) {
					rep.Actions = append(rep.Actions, Action{Kind: "base-lost", Name: n})
					continue
				}
				if tree, err = repo.TreeSHA(e.Commit, e.Path); err != nil {
					return Report{}, err
				}
				if tree != e.Tree {
					return Report{}, fmt.Errorf("skill %s: tree at commit %s is %s, but the lock says %s", n, e.Commit, tree, e.Tree)
				}
			}
			et, err := exportTree(repo, tree, n) // only after the check: no blobs fetched for a mismatched tree
			if errors.Is(err, skillfs.ErrInvalid) {
				skip(n, err)
				continue
			}
			if err != nil {
				return Report{}, err
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
			continue // base-lost or ref changed
		}
		fresh := slices.Contains(missing, n)
		tgs := m.TargetsFor(n)
		if len(tgs) == 0 {
			return Report{}, fmt.Errorf("skill %s: no targets: set targets in %s", n, sc.manifest)
		}
		var skillJobs []job
		var skillActs []Action
		unmanaged := false
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
				skillJobs = append(skillJobs, job{n, tg, final, t})
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
				skillActs = append(skillActs, Action{Kind: "adopted", Name: n, Target: tg})
			case same:
			case fresh:
				// Same rule as add: an unmanaged folder that differs is never taken over.
				rep.Actions = append(rep.Actions, Action{Kind: "kept", Name: n, Target: tg, Detail: "unmanaged"})
				unmanaged = true
			default:
				rep.Actions = append(rep.Actions, Action{Kind: "kept", Name: n, Target: tg, Detail: "modified"})
			}
		}
		if unmanaged {
			// The skill stays unlocked and is not installed or adopted anywhere this run.
			delete(newL.Skills, n)
			continue
		}
		jobs = append(jobs, skillJobs...)
		rep.Actions = append(rep.Actions, skillActs...)
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
	skipped := false
	for _, a := range rep.Actions {
		_, _ = fmt.Fprintln(out, a.String())
		skipped = skipped || a.Kind == "skipped"
	}
	if skipped {
		return rep, ErrSkipped
	}
	return rep, nil
}

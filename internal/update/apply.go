package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rojcarranza/cali/internal/backup"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/merge"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// job replaces one installed folder with next.
type job struct {
	skill, target, dir string
	exists             bool
	ours, base, next   skillfs.Tree
	lost               bool // base lost: back up the whole folder, no local.patch
}

// swapped is a folder already replaced; old is where the previous one waits
// (empty when there was none) and ours is what it held when it was backed up.
type swapped struct {
	dir, old string
	ours     skillfs.Tree
}

// beforeCleanup, when set by a test, runs before a moved-aside folder is removed.
var beforeCleanup func(old string)

// beforeLockWrite, when set by a test, runs after the swaps and before the lock is re-read.
var beforeLockWrite func()

// Apply writes the update planned in p (design §5 steps 4 to 6). Every
// conflict must be resolved, by p.Opts or by prompt (nil: never asks), before
// anything is written; otherwise nothing is written and the error lists the
// open conflicts. A skill skipped (gone upstream, invalid upstream, not locked) is reported and,
// after the rest is written, makes Apply return install.ErrSkipped. Folders with local edits are backed up first, each folder
// is swapped in by rename and restored on any failure, and the lock is
// written last.
func Apply(ctx context.Context, d install.Deps, p Proposal, prompt Prompter) error {
	out := d.Stdout
	if out == nil {
		out = io.Discard
	}
	o := p.Opts
	loc, err := install.Locate(d, d.Global)
	if err != nil {
		return err
	}
	if err := checkFlags(p); err != nil {
		return err
	}
	g := p.flags

	var jobs []job
	var advanced, refOnly []SkillPlan
	var kept, open []string
	for _, s := range p.Skills {
		switch {
		case s.Problem != "":
			kept = append(kept, s.Name+": skipped: "+render.Sanitize(s.Problem))
			continue
		case s.Noop:
			if s.ref != s.fromRef {
				refOnly = append(refOnly, s) // same folder at the new ref: only the lock changes
			}
			continue
		case s.BaseLost && o.KeepMine:
			kept = append(kept, s.Name+": base lost, kept the installed folder")
			continue
		case s.BaseLost && !o.TakeUpstream:
			open = append(open, "  "+s.Name+": "+baseLostHint)
			continue
		case s.BaseLost:
			for _, tp := range s.lost {
				jobs = append(jobs, job{skill: s.Name, target: tp.Target, dir: tp.Dir, exists: !tp.Missing, ours: tp.ours, next: s.theirs, lost: true})
			}
			advanced = append(advanced, s)
			continue
		}
		sj, skip, unres, err := resolveSkill(s, o, prompt, g)
		if err != nil {
			return err
		}
		switch {
		case skip != "":
			kept = append(kept, s.Name+": "+skip)
		case len(unres) > 0:
			open = append(open, unres...)
		default:
			jobs = append(jobs, sj...)
			advanced = append(advanced, s)
		}
	}
	if len(open) > 0 {
		return fmt.Errorf("update stopped, nothing was written: resolve every conflict first\n%s", strings.Join(open, "\n"))
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Safety checks for every folder before the first write.
	for i, j := range jobs {
		if loc.Scope == targets.Project {
			if err := install.NoSymlinks(loc.Root, j.dir); err != nil {
				return err
			}
		}
		fi, err := os.Lstat(j.dir)
		switch {
		case !j.exists && errors.Is(err, os.ErrNotExist):
			continue
		case !j.exists && err == nil:
			return fmt.Errorf("%s appeared while updating; not overwriting it", j.dir)
		case err != nil:
			return err
		case !fi.IsDir():
			return fmt.Errorf("%s is not a directory", j.dir)
		}
		cur, err := skillfs.ReadDir(j.dir)
		if err != nil {
			return err
		}
		if !skillfs.Equal(cur, j.ours) {
			return fmt.Errorf("%s changed while updating; run cali update again", j.dir)
		}
		jobs[i].ours = cur // back up the bytes on disk now
	}

	// No local edit is lost: back up every folder that differs from its base.
	for _, j := range jobs {
		if !j.exists || (!j.lost && skillfs.SameBytes(j.base, j.ours)) { // a line-ending-only edit is still an edit
			continue
		}
		var patch []byte
		if !j.lost {
			if patch, err = merge.LocalPatch(j.base, j.ours); err != nil {
				return fmt.Errorf("local patch of %s: %w", j.dir, err)
			}
		}
		dir, err := backup.Save(d.Dirs.Data, j.skill, j.ours, patch)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "%s (%s) backup: %s\n", j.skill, j.target, dir)
	}

	for _, s := range advanced {
		if err := store.Put(d.Dirs.Data, s.ToTree, s.theirs); err != nil {
			return fmt.Errorf("store pristine copy of %s: %w", s.Name, err)
		}
	}

	rename := d.Rename
	if rename == nil {
		rename = os.Rename
	}
	var done []swapped
	restore := func(cause error) error {
		errs := []error{cause}
		for i := len(done) - 1; i >= 0; i-- {
			sw := done[i]
			if err := os.RemoveAll(sw.dir); err != nil {
				errs = append(errs, err)
				continue
			}
			if sw.old != "" {
				if err := rename(sw.old, sw.dir); err != nil {
					errs = append(errs, fmt.Errorf("restore %s failed, the previous folder is at %s: %w", sw.dir, sw.old, err))
				}
			}
		}
		return errors.Join(errs...)
	}
	root := "" // global scope may use a symlinked skills dir (as add allows)
	if loc.Scope == targets.Project {
		root = loc.Root
	}
	for _, j := range jobs {
		sw, err := swap(j, rename, root)
		if err != nil {
			return restore(err)
		}
		done = append(done, sw)
	}

	if beforeLockWrite != nil {
		beforeLockWrite()
	}
	l, err := lock.Read(loc.LockPath)
	if err != nil {
		return restore(err)
	}
	for _, s := range append(advanced, refOnly...) {
		e, ok := l.Skills[s.Name]
		if !ok {
			return restore(fmt.Errorf("%s was removed from cali.lock during the update; nothing written", s.Name))
		}
		e.Ref, e.Commit, e.Tree = s.ref, install.LockCommit(e.Source, s.ToCommit), s.ToTree
		l.Skills[s.Name] = e
	}
	if len(advanced) > 0 || len(refOnly) > 0 {
		if err := lock.Write(loc.LockPath, l); err != nil {
			return restore(err)
		}
	}
	for _, sw := range done {
		if sw.old != "" {
			if beforeCleanup != nil {
				beforeCleanup(sw.old)
			}
			// An edit saved into old after the swap is not in the backup: keep it.
			if cur, err := skillfs.ReadDir(sw.old); err != nil || !skillfs.SameBytes(cur, sw.ours) {
				_, _ = fmt.Fprintf(out, "kept %s: it changed during the update\n", sw.old)
				continue
			}
			_ = os.RemoveAll(sw.old) // best effort: the update itself is complete
		}
	}
	for _, s := range advanced {
		_, _ = fmt.Fprintf(out, "updated %s %s -> %s\n", s.Name, short(s.FromCommit), short(s.ToCommit))
	}
	for _, k := range kept {
		_, _ = fmt.Fprintf(out, "kept %s\n", k)
	}
	if p.Skipped() {
		return install.ErrSkipped
	}
	return nil
}

// checkFlags refuses a --resolve naming no conflict, and --accept-recommended
// when a conflict it would decide has no safe recommendation.
func checkFlags(p Proposal) error {
	o := p.Opts
	keys := map[string]bool{}
	var unsafe []string
	for _, s := range p.Skills {
		for _, t := range s.Targets {
			for _, f := range t.Files {
				if f.Action != merge.Conflicted {
					continue
				}
				key := s.Name + ":" + f.Path
				keys[key] = true
				if _, ok := o.Resolve[key]; !ok && o.AcceptRecommended && !o.TakeUpstream && !o.KeepMine && !f.Rec.Safe {
					unsafe = append(unsafe, fmt.Sprintf("  %s %s %s [%s]", s.Name, t.Target, render.Sanitize(f.Path), f.Rec.Class))
				}
			}
		}
	}
	for k := range o.Resolve {
		if !keys[k] {
			return fmt.Errorf("--resolve %s: that file has no conflict", render.Sanitize(k))
		}
	}
	if len(unsafe) > 0 {
		return fmt.Errorf("--accept-recommended refused, nothing was written: these conflicts need your decision\n%s", strings.Join(unsafe, "\n"))
	}
	return nil
}

// resolveSkill builds the new folder of every target of s. skip is set when
// the skill must not advance (a conflict kept without an explicit choice, or
// skipped at the prompt); unres lists conflicts nothing decided.
func resolveSkill(s SkillPlan, o Options, prompt Prompter, g string) (jobs []job, skip string, unres []string, err error) {
	tm := skillfs.Index(s.theirs)
	for _, tp := range s.Targets {
		j := job{skill: s.Name, target: tp.Target, dir: tp.Dir, exists: !tp.Missing, ours: tp.ours, base: s.base, next: s.theirs}
		if tp.Missing {
			jobs = append(jobs, j)
			continue
		}
		om := skillfs.Index(tp.ours)
		open := len(unres)
		next := skillfs.Tree{}
		put := func(f *skillfs.File) {
			if f != nil {
				next = append(next, *f)
			}
		}
		for _, f := range tp.Files {
			if f.Action != merge.Conflicted {
				if !f.Delete {
					next = append(next, skillfs.File{Path: f.Path, Exec: f.Exec, Data: f.Merged})
				}
				continue
			}
			choice, explicit := o.Resolve[s.Name+":"+f.Path]
			switch {
			case explicit:
			case o.TakeUpstream:
				choice = "theirs"
			case o.KeepMine:
				choice = "ours"
			case o.AcceptRecommended:
				choice = f.Rec.Choice
			case prompt != nil:
				// Not explicit: keep-mine at the prompt keeps the skill, only --resolve f=ours advances it.
				if choice, err = prompt.Choose(s.Name, f.Path, *f.Conflict, *f.Rec); err != nil {
					return nil, "", nil, err
				}
			}
			path := render.Sanitize(f.Path)
			switch choice {
			case "":
				unres = append(unres, fmt.Sprintf("  %s %s %s: RECOMMEND %s [%s] (%s)\n    %s",
					s.Name, tp.Target, path, f.Rec.Choice, f.Rec.Class, f.Rec.Reason, resolveHint(s.Name, g, f.Path, f.Rec.Choice)))
			case "skip":
				return nil, "skipped", nil, nil
			case "theirs":
				put(tm[f.Path])
			case "ours":
				if !explicit {
					// Advancing the lock would hide the upstream change from later merges.
					return nil, "kept local files for its conflicts; resolve them with --resolve <file>=ours to advance", nil, nil
				}
				put(om[f.Path])
			case "merge":
				res, ok := merge.ResolveDisjoint(*f.Conflict)
				if !ok {
					return nil, "", nil, fmt.Errorf("%s %s: the changes overlap and cannot be merged; choose ours or theirs", s.Name, path)
				}
				exe := false
				if of := om[f.Path]; of != nil {
					exe = of.Exec
				}
				next = append(next, skillfs.File{Path: f.Path, Exec: exe, Data: merge.EncodeLike(f.Conflict.Ours, res)})
			default:
				return nil, "", nil, fmt.Errorf("%s %s: unknown choice %q", s.Name, path, choice)
			}
		}
		if len(unres) > open {
			continue // open conflicts: this folder is never written, so it is not built
		}
		if err := skillfs.Validate(next); err != nil {
			return nil, "", nil, fmt.Errorf("skill %s (%s): new folder is not valid: %w", s.Name, tp.Target, err)
		}
		if skillfs.Equal(next, tp.ours) {
			continue // nothing to write in this folder
		}
		j.next = next
		jobs = append(jobs, j)
	}
	return jobs, "", unres, nil
}

// swap stages j.next beside j.dir and renames it into place, moving the old
// folder to a reserved .cali-old-* path the caller removes or restores. With
// root set (project scope) it refuses a symlink anywhere from root to the
// parent, checked again here because the tree may have changed since planning.
func swap(j job, rename func(string, string) error, root string) (swapped, error) {
	sw := swapped{dir: j.dir, ours: j.ours}
	parent := filepath.Dir(j.dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return sw, err
	}
	if root != "" {
		if err := install.NoSymlinks(root, parent); err != nil {
			return sw, err
		}
	}
	staging, err := install.Stage(parent, j.next)
	if err != nil {
		return sw, err
	}
	if j.exists {
		old, err := install.MoveAside(j.dir, rename)
		if err != nil {
			_ = os.RemoveAll(staging)
			return sw, err
		}
		// An edit saved after the backup would be deleted with old: put it back and stop.
		if cur, err := skillfs.ReadDir(old); err != nil || !skillfs.SameBytes(cur, j.ours) {
			_ = os.RemoveAll(staging)
			if rerr := rename(old, j.dir); rerr != nil {
				return sw, fmt.Errorf("%s changed while updating and could not be moved back; it is at %s: %w", j.dir, old, rerr)
			}
			return sw, fmt.Errorf("%s changed while updating; left as is, run cali update again", j.dir)
		}
		sw.old = old
	}
	if err := rename(staging, j.dir); err != nil {
		_ = os.RemoveAll(staging)
		err = fmt.Errorf("move %s into place: %w", j.dir, err)
		if sw.old != "" {
			if rerr := rename(sw.old, j.dir); rerr != nil {
				return sw, errors.Join(err, fmt.Errorf("restore %s failed, the previous folder is at %s: %w", j.dir, sw.old, rerr))
			}
		}
		return sw, err
	}
	return sw, nil
}

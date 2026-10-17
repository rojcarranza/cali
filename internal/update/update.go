// Package update plans upstream updates of installed skills as a three-way
// merge per target folder (design §5). Plan only reads: it writes nothing to
// skill folders, the lock, the manifest, the store or backups.
package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/merge"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// Options is one `cali update` invocation.
type Options struct {
	Names                                     []string
	Apply                                     bool
	Resolve                                   map[string]string // "skill:file" -> "ours"|"theirs"|"merge"
	TakeUpstream, KeepMine, AcceptRecommended bool
}

// Proposal is the planned update of every selected skill, and the options
// Apply resolves conflicts with.
type Proposal struct {
	Skills []SkillPlan
	Opts   Options
	flags  string // scope flags (-g, --manifest) for the printed resolve commands
}

// SkillPlan is one skill: Noop when upstream did not change its folder,
// BaseLost when the pristine copy of the locked tree is gone.
type SkillPlan struct {
	Name                                   string
	FromCommit, ToCommit, FromTree, ToTree string
	BaseLost                               bool
	Noop                                   bool
	Targets                                []TargetPlan
	base, theirs                           skillfs.Tree
	ref, fromRef                           string       // effective manifest ref (written to the lock) and the locked one
	lost                                   []TargetPlan // BaseLost only: the folders --take-upstream replaces
}

// TargetPlan is one installed folder; Missing when it does not exist.
type TargetPlan struct {
	Target, Dir string
	Missing     bool
	Files       []FileDecision
	ours        skillfs.Tree
}

// FileDecision is a merge.FilePlan plus the pre-resolution or recommendation
// for a conflict. LocalLines and UpstreamLines are base-to-side changed lines.
type FileDecision struct {
	merge.FilePlan
	PreResolved               bool
	Rec                       *merge.Recommendation
	LocalLines, UpstreamLines int
	same                      bool // base, ours and theirs are equal: nothing to show
}

// Prompter asks the user to resolve one conflict (TTY only; nil otherwise).
type Prompter interface {
	Choose(skill, file string, c merge.Conflict, rec merge.Recommendation) (choice string, err error)
}

// Plan computes what an update would do for o.Names (all manifest skills of
// the scope when empty). It fetches each source once and writes nothing
// outside the cache mirror.
func Plan(ctx context.Context, d install.Deps, o Options) (Proposal, error) {
	opts := source.Options{AllowFile: d.AllowFileScheme}
	note := d.Stdout
	if note == nil {
		note = io.Discard
	}
	loc, err := install.Locate(d, d.Global)
	if err != nil {
		return Proposal{}, err
	}
	m, err := manifest.ReadWith(loc.Manifest, opts)
	noManifest := errors.Is(err, os.ErrNotExist)
	if noManifest {
		m, err = manifest.Manifest{Version: 1, Skills: map[string]manifest.Skill{}}, nil
	}
	if err != nil {
		return Proposal{}, err
	}
	l, err := lock.Read(loc.LockPath)
	if err != nil {
		return Proposal{}, err
	}
	if noManifest && len(l.Skills) > 0 {
		return Proposal{}, fmt.Errorf("%s not found but %s has skills; restore the manifest first", loc.Manifest, loc.LockPath)
	}
	names := slices.Clone(o.Names)
	if len(names) == 0 {
		for n := range m.Skills {
			names = append(names, n)
		}
	}
	for _, n := range names {
		if _, ok := m.Skills[n]; !ok {
			return Proposal{}, fmt.Errorf("skill %q is not in %s", n, loc.Manifest)
		}
	}
	sort.Strings(names)
	names = slices.Compact(names)

	repos := map[string]*gitx.Repo{}
	warned := map[string]bool{} // skills dirs already checked for leftovers
	p := Proposal{Opts: o}
	if d.Global {
		p.flags = " -g"
	}
	if d.Manifest != "" {
		p.flags += " --manifest " + shellArg(loc.Manifest)
	}
	for _, n := range names {
		s := m.Skills[n]
		spec, err := source.ParseWith(s.Source, opts)
		if err != nil {
			return Proposal{}, fmt.Errorf("skill %s: %w", n, err)
		}
		// As in sync: a lock entry for another origin or path does not count;
		// update never moves a skill to another origin or path.
		e, ok := l.Skills[n]
		if !ok {
			_, _ = fmt.Fprintf(note, "%s: not locked, run cali sync\n", n)
			continue
		}
		if e.Source != spec.URL || e.Path != strings.Trim(spec.Path, "/") {
			return Proposal{}, fmt.Errorf("%s: source or path changed in cali.yaml; run cali remove %s and cali add again", n, n)
		}
		for _, tg := range m.TargetsFor(n) { // also for up-to-date skills: the leftover may be from any run
			dir, err := targets.Dir(tg, loc.Scope, loc.Root, d.Dirs.Home)
			if err != nil {
				return Proposal{}, err
			}
			if !warned[dir] {
				warned[dir] = true
				warnLeftovers(note, dir)
			}
		}
		ref := s.Ref
		if ref == "" {
			ref = spec.Ref
		}
		repo, ok := repos[spec.URL]
		if !ok {
			if repo, err = install.OpenFetch(ctx, d, spec); err != nil {
				return Proposal{}, err
			}
			repos[spec.URL] = repo
		}
		commit, err := repo.Resolve(ref)
		if err != nil {
			return Proposal{}, fmt.Errorf("skill %s: %w", n, err)
		}
		sp := SkillPlan{Name: n, FromCommit: e.Commit, ToCommit: commit, FromTree: e.Tree, ref: ref, fromRef: e.Ref}
		if sp.ToTree, err = repo.TreeSHA(commit, e.Path); err != nil {
			return Proposal{}, fmt.Errorf("skill %s: %w", n, err)
		}
		if sp.ToTree == e.Tree {
			sp.Noop = true
			p.Skills = append(p.Skills, sp)
			continue
		}
		base, ok, err := store.Get(d.Dirs.Data, e.Tree)
		if err != nil {
			return Proposal{}, fmt.Errorf("pristine copy of %s: %w", n, err)
		}
		_, theirs, err := install.ExportSkill(repo, commit, e.Path, n)
		if err != nil {
			return Proposal{}, err
		}
		sp.BaseLost, sp.base, sp.theirs = !ok, base, theirs
		for _, tg := range m.TargetsFor(n) {
			dir, err := targets.Dir(tg, loc.Scope, loc.Root, d.Dirs.Home)
			if err != nil {
				return Proposal{}, err
			}
			tp := TargetPlan{Target: tg, Dir: filepath.Join(dir, n)}
			fi, err := os.Lstat(tp.Dir)
			switch {
			case errors.Is(err, os.ErrNotExist):
				tp.Missing = true
			case err != nil:
				return Proposal{}, err
			case !fi.IsDir(): // Lstat: a symlink is not a dir either
				return Proposal{}, fmt.Errorf("%s is not a directory", tp.Dir)
			default:
				if tp.ours, err = skillfs.ReadDir(tp.Dir); err != nil {
					return Proposal{}, err
				}
				if sp.BaseLost {
					break
				}
				if tp.Files, err = decide(base, tp.ours, theirs); err != nil {
					return Proposal{}, fmt.Errorf("skill %s (%s): %w", n, tg, err)
				}
			}
			if sp.BaseLost {
				sp.lost = append(sp.lost, tp)
			} else {
				sp.Targets = append(sp.Targets, tp)
			}
		}
		p.Skills = append(p.Skills, sp)
	}
	return p, nil
}

// warnLeftovers reports staging and moved-aside folders an interrupted update
// left in the skills dir. They may hold the only copy of a folder, so they are
// never deleted.
func warnLeftovers(w io.Writer, dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return // absent or unreadable: nothing to report
	}
	for _, e := range ents {
		if name := e.Name(); strings.HasPrefix(name, ".cali-old-") || strings.HasPrefix(name, ".cali-staging-") {
			_, _ = fmt.Fprintf(w, "warning: leftover from an interrupted update: %s\n", render.Sanitize(filepath.Join(dir, name)))
		}
	}
}

func index(t skillfs.Tree) map[string]*skillfs.File {
	m := make(map[string]*skillfs.File, len(t))
	for i := range t {
		m[t[i].Path] = &t[i]
	}
	return m
}

func eq(a, b *skillfs.File) bool {
	if a == nil || b == nil {
		return a == b
	}
	return skillfs.FileEqual(*a, *b)
}

func text(sides ...[]byte) bool {
	for _, b := range sides {
		if !skillfs.IsText(b) {
			return false
		}
	}
	return true
}

// decide runs merge.Plan, pre-resolves whitespace-only conflicts and
// classifies the rest.
func decide(base, ours, theirs skillfs.Tree) ([]FileDecision, error) {
	fps, err := merge.Plan(base, ours, theirs)
	if err != nil {
		return nil, err
	}
	bm, om, tm := index(base), index(ours), index(theirs)
	out := make([]FileDecision, 0, len(fps))
	for _, fp := range fps {
		fd := FileDecision{FilePlan: fp, same: eq(bm[fp.Path], om[fp.Path]) && eq(om[fp.Path], tm[fp.Path])}
		if fp.Action == merge.Conflicted {
			c := *fp.Conflict
			if res, ok := merge.PreResolve(c.Base, c.Ours, c.Theirs); ok {
				fd.PreResolved, fd.Conflict = true, nil
				switch {
				case res == nil && c.Ours == nil: // local deleted it, upstream only touched whitespace: keep the deletion
					fd.Action, fd.Delete = merge.KeepLocal, true
				case res == nil: // upstream deleted it, local only touched whitespace
					fd.Action, fd.Delete = merge.TakeUpstream, true
				case bytes.Equal(res, c.Ours) && !bytes.Equal(res, c.Theirs):
					fd.Action, fd.Merged, fd.Exec = merge.KeepLocal, res, om[fp.Path].Exec
				default:
					fd.Action, fd.Merged, fd.Exec = merge.TakeUpstream, res, tm[fp.Path].Exec
				}
			} else {
				if text(c.Base, c.Ours, c.Theirs) {
					if fd.LocalLines, err = merge.ChangedLines(c.Base, c.Ours); err != nil {
						return nil, err
					}
					if fd.UpstreamLines, err = merge.ChangedLines(c.Base, c.Theirs); err != nil {
						return nil, err
					}
				}
				rec := merge.Classify(fp.Path, c, fd.LocalLines, fd.UpstreamLines)
				fd.Rec = &rec
			}
		}
		out = append(out, fd)
	}
	return out, nil
}

// Conflicts counts the conflicting files over all skills and targets.
func (p Proposal) Conflicts() int {
	n := 0
	for _, s := range p.Skills {
		for _, t := range s.Targets {
			for _, f := range t.Files {
				if f.Action == merge.Conflicted {
					n++
				}
			}
		}
	}
	return n
}

// baseLostHint is what dry run and apply say for a lost base: sync can often rebuild it.
const baseLostHint = "base lost: run cali sync to rebuild it; if sync reports base-lost, use --take-upstream"

func short(c string) string { return c[:min(7, len(c))] }

// shellArg quotes s for a POSIX shell unless it is plainly safe: paths come
// from the repo and the printed command is meant to be pasted.
func shellArg(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._/-=") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// WriteText prints the dry run: per skill a header, per changed file its
// action, and for each conflict the hunks, the recommendation and the
// command that applies it, with the scope flags Plan was run with.
func (p Proposal) WriteText(w io.Writer) error {
	var b strings.Builder
	g := p.flags
	n := 0
	for _, s := range p.Skills {
		switch {
		case s.Noop:
			fmt.Fprintf(&b, "%s: up to date\n", s.Name)
			continue
		case s.BaseLost:
			n++
			fmt.Fprintf(&b, "%s: %s\n", s.Name, baseLostHint)
			continue
		}
		n++
		fmt.Fprintf(&b, "%s %s -> %s\n", s.Name, short(s.FromCommit), short(s.ToCommit))
		for _, t := range s.Targets {
			if t.Missing {
				fmt.Fprintf(&b, "  %s: missing, upstream will be installed\n", t.Target)
				continue
			}
			for _, f := range t.Files {
				if f.same {
					continue
				}
				path := render.Sanitize(f.Path)
				if f.Action != merge.Conflicted {
					fmt.Fprintf(&b, "  %s %s: %s\n", t.Target, path, f.Action)
					continue
				}
				fmt.Fprintf(&b, "  %s %s: CONFLICT\n", t.Target, path)
				for _, h := range f.Conflict.Hunks {
					fmt.Fprintf(&b, "    @@ ours line %d\n", h.OursStart)
					for _, side := range []struct {
						tag   string
						lines []string
					}{{"ours", h.Ours}, {"base", h.Base}, {"theirs", h.Theirs}} {
						for _, l := range side.lines {
							fmt.Fprintf(&b, "    %s| %s\n", side.tag, render.Sanitize(l))
						}
					}
				}
				fmt.Fprintf(&b, "    RECOMMEND %s [%s] (%s)\n", f.Rec.Choice, f.Rec.Class, f.Rec.Reason)
				fmt.Fprintf(&b, "    %s\n", resolveHint(s.Name, g, f.Path, f.Rec.Choice))
			}
		}
	}
	fmt.Fprintf(&b, "%d skill(s) to update, %d conflict(s)\n", n, p.Conflicts())
	_, err := io.WriteString(w, b.String())
	return err
}

type jsonHunk struct {
	OursStart int      `json:"ours_start"`
	Ours      []string `json:"ours"`
	Base      []string `json:"base"`
	Theirs    []string `json:"theirs"`
}

type jsonConflict struct {
	Class         merge.Class `json:"class"`
	Choice        string      `json:"choice"`
	Reason        string      `json:"reason"`
	Safe          bool        `json:"safe"`
	LocalLines    int         `json:"local_lines"`
	UpstreamLines int         `json:"upstream_lines"`
	Hunks         []jsonHunk  `json:"hunks"`
}

type jsonFile struct {
	Path     string        `json:"path"`
	Action   merge.Action  `json:"action"`
	Delete   bool          `json:"delete"`
	Conflict *jsonConflict `json:"conflict,omitempty"`
}

type jsonTarget struct {
	Target  string     `json:"target"`
	Dir     string     `json:"dir"`
	Missing bool       `json:"missing"`
	Files   []jsonFile `json:"files"`
}

type jsonSkill struct {
	Name     string       `json:"name"`
	From     string       `json:"from"`
	To       string       `json:"to"`
	Noop     bool         `json:"noop"`
	BaseLost bool         `json:"base_lost"`
	Targets  []jsonTarget `json:"targets"`
}

// WriteJSON prints the dry run as JSON (render.WriteJSON adds schema_version).
func (p Proposal) WriteJSON(w io.Writer) error {
	skills := []jsonSkill{}
	for _, s := range p.Skills {
		js := jsonSkill{Name: s.Name, From: s.FromCommit, To: s.ToCommit, Noop: s.Noop, BaseLost: s.BaseLost, Targets: []jsonTarget{}}
		for _, t := range s.Targets {
			jt := jsonTarget{Target: t.Target, Dir: t.Dir, Missing: t.Missing, Files: []jsonFile{}}
			for _, f := range t.Files {
				jf := jsonFile{Path: f.Path, Action: f.Action, Delete: f.Delete}
				if f.Action == merge.Conflicted {
					jc := &jsonConflict{Class: f.Rec.Class, Choice: f.Rec.Choice, Reason: f.Rec.Reason, Safe: f.Rec.Safe,
						LocalLines: f.LocalLines, UpstreamLines: f.UpstreamLines, Hunks: []jsonHunk{}}
					for _, h := range f.Conflict.Hunks {
						jc.Hunks = append(jc.Hunks, jsonHunk{h.OursStart, append([]string{}, h.Ours...),
							append([]string{}, h.Base...), append([]string{}, h.Theirs...)})
					}
					jf.Conflict = jc
				}
				jt.Files = append(jt.Files, jf)
			}
			js.Targets = append(js.Targets, jt)
		}
		skills = append(skills, js)
	}
	return render.WriteJSON(w, struct {
		Skills []jsonSkill `json:"skills"`
	}{skills})
}

// resolveHint is the command that resolves one conflict. A path Sanitize
// changes cannot be typed back to match --resolve, so it gets the whole-skill flags instead.
func resolveHint(skill, g, path, choice string) string {
	if render.Sanitize(path) != path {
		return "file name has control characters; resolve it with --take-upstream or --keep-mine"
	}
	return fmt.Sprintf("cali update %s%s --apply --resolve %s", skill, g, shellArg(path+"="+choice))
}

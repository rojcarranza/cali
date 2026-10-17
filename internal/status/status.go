// Package status works out the state of each installed skill per target:
// local edits, newer upstream trees, missing folders and dependencies.
package status

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/rojcarranza/cali/internal/deps"
	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// State is one finding about a skill in a target.
type State string

// States a row can carry.
const (
	OK        State = "ok"
	Modified  State = "modified"
	Outdated  State = "outdated"
	Missing   State = "missing"
	BaseLost  State = "base-lost"
	Unmanaged State = "unmanaged"
	Shadowed  State = "shadowed"
)

// Row is the status of one skill in one target.
type Row struct {
	Skill  string  `json:"skill"`
	Target string  `json:"target"`
	States []State `json:"states"`
	Detail string  `json:"detail"`
}

// Fetcher returns the latest commit of spec.Ref (empty = default branch) and
// the tree SHA of spec.Path at it.
type Fetcher interface {
	Latest(spec source.Spec) (commit, tree string, err error)
}

// Input is what Compute looks at.
type Input struct {
	Scope               targets.Scope
	Root, Home, DataDir string // project root (empty for global), home, cali data dir
	Manifest            manifest.Manifest
	Lock                lock.Lock
	Global              manifest.Manifest // the global manifest; used in project scope only
	Offline             bool
	Fetcher             Fetcher
	SourceOptions       source.Options
	Warn                io.Writer // fetch warnings; nil discards them
}

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Compute returns one row per manifest skill x target (sorted by skill, then
// target), plus unmanaged and shadowed rows, and the dependency warnings.
func Compute(ctx context.Context, in Input) ([]Row, []deps.Warning, error) {
	warn := in.Warn
	if warn == nil {
		warn = io.Discard
	}
	rows := []Row{}
	installed := map[string][]string{}
	requires := map[string][]string{}
	warned := map[string]bool{}

	for _, name := range sortedKeys(in.Manifest.Skills) {
		sk := in.Manifest.Skills[name]
		e, locked := in.Lock.Skills[name]
		outdated := false
		if locked && !in.Offline && in.Fetcher != nil {
			spec, err := source.ParseWith(sk.Source, in.SourceOptions)
			if err != nil {
				return nil, nil, fmt.Errorf("skill %s: %w", name, err)
			}
			ref := sk.Ref // the effective ref, as sync and update use it
			if ref == "" {
				ref = spec.Ref
			}
			spec.Ref, spec.Path = ref, e.Path
			if !fullSHA.MatchString(ref) { // a commit pin is never outdated: nothing to fetch
				if err := ctx.Err(); err != nil {
					return nil, nil, err
				}
				if _, tree, err := in.Fetcher.Latest(spec); err != nil {
					if !warned[spec.URL] {
						warned[spec.URL] = true
						_, _ = fmt.Fprintf(warn, "warning: %s: %v\n", spec.URL, err)
					}
				} else {
					outdated = tree != e.Tree
				}
			}
		}

		var fm skillfs.Frontmatter
		for _, tg := range in.Manifest.TargetsFor(name) {
			base, err := targets.Dir(tg, in.Scope, in.Root, in.Home)
			if err != nil {
				return nil, nil, err
			}
			r := Row{Skill: name, Target: tg}
			have, present, odd, err := readFolder(filepath.Join(base, name))
			if err != nil {
				return nil, nil, err
			}
			switch {
			case odd != "":
				r.States, r.Detail = append(r.States, Modified), odd
			case !present:
				r.States = append(r.States, Missing)
			case !locked:
				r.States, r.Detail = append(r.States, BaseLost), "run cali sync"
			default:
				pristine, ok, err := store.Get(in.DataDir, e.Tree)
				if err != nil {
					return nil, nil, fmt.Errorf("skill %s: %w", name, err)
				}
				if !ok {
					r.States, r.Detail = append(r.States, BaseLost), "run cali sync"
				} else if !skillfs.Equal(have, pristine) {
					r.States = append(r.States, Modified)
				}
			}
			if outdated {
				r.States = append(r.States, Outdated)
			}
			if len(r.States) == 0 {
				r.States = []State{OK}
			}
			rows = append(rows, r)
			if present && odd == "" {
				installed[tg] = append(installed[tg], name)
				if fm.Name == "" {
					fm = frontmatter(have)
				}
			}
		}
		requires[name] = deps.Requires(sk, fm)
	}

	if in.Scope == targets.Project {
		for _, name := range sortedKeys(in.Global.Skills) {
			for _, tg := range in.Global.TargetsFor(name) {
				base, err := targets.Dir(tg, targets.Global, "", in.Home)
				if err != nil {
					return nil, nil, err
				}
				if fi, err := os.Stat(filepath.Join(base, name)); err == nil && fi.IsDir() && !slices.Contains(installed[tg], name) {
					installed[tg] = append(installed[tg], name)
				}
				if _, ok := in.Manifest.Skills[name]; ok && slices.Contains(in.Manifest.TargetsFor(name), tg) {
					rows = append(rows, Row{Skill: name, Target: tg, States: []State{Shadowed}, Detail: "global"})
				}
			}
		}
	}

	um, err := UnmanagedRows(in.Manifest, in.Scope, in.Root, in.Home)
	if err != nil {
		return nil, nil, err
	}
	rows = append(rows, um...)

	// Stable: a project row stays ahead of the shadowed global row of the same skill and target.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Skill != rows[j].Skill {
			return rows[i].Skill < rows[j].Skill
		}
		return rows[i].Target < rows[j].Target
	})
	return rows, deps.Check(installed, requires), nil
}

// UnmanagedRows returns an Unmanaged row for every folder holding a regular
// SKILL.md in the skills dir of any target m uses, that m does not manage.
// Names starting with "." (Cali's staging and moved-aside folders) are skipped.
func UnmanagedRows(m manifest.Manifest, scope targets.Scope, root, home string) ([]Row, error) {
	used := slices.Clone(m.Targets)
	for _, name := range sortedKeys(m.Skills) {
		for _, tg := range m.TargetsFor(name) {
			if !slices.Contains(used, tg) {
				used = append(used, tg)
			}
		}
	}
	var rows []Row
	for _, tg := range used {
		base, err := targets.Dir(tg, scope, root, home)
		if err != nil {
			return nil, err
		}
		ents, err := os.ReadDir(base)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, ent := range ents {
			name := ent.Name()
			if _, managed := m.Skills[name]; managed || !ent.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			if fi, err := os.Lstat(filepath.Join(base, name, "SKILL.md")); err == nil && fi.Mode().IsRegular() {
				rows = append(rows, Row{Skill: name, Target: tg, States: []State{Unmanaged}})
			}
		}
	}
	return rows, nil
}

// readFolder reads an installed skill folder; present is false when it does
// not exist, and odd says why a present path is not a folder Cali manages or
// cannot be read.
func readFolder(dir string) (t skillfs.Tree, present bool, odd string, err error) {
	fi, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, "", nil
	}
	if err != nil {
		return nil, false, "", err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return nil, true, "symlink, not managed by Cali", nil
	case !fi.IsDir():
		return nil, true, "not a directory", nil
	}
	t, err = skillfs.ReadDir(dir)
	if err != nil {
		// One unreadable folder (say a symlink inside it) must not hide every other row.
		return nil, true, render.Sanitize(err.Error()), nil
	}
	return t, true, "", nil
}

// frontmatter parses the SKILL.md of t; an unparsable one counts as empty, as in show.
func frontmatter(t skillfs.Tree) skillfs.Frontmatter {
	for _, f := range t {
		if f.Path == "SKILL.md" {
			fm, _ := skillfs.ParseFrontmatter(f.Data)
			return fm
		}
	}
	return skillfs.Frontmatter{}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GitFetcher is the real Fetcher: it fetches each source URL's mirror once
// per run and resolves the ref in it.
type GitFetcher struct {
	Ctx      context.Context
	CacheDir string
	Getenv   func(string) string
	GhToken  func() (string, error)

	repos map[string]*gitx.Repo
	errs  map[string]error
}

// Latest implements Fetcher.
func (f *GitFetcher) Latest(s source.Spec) (string, string, error) {
	if f.repos == nil {
		f.repos, f.errs = map[string]*gitx.Repo{}, map[string]error{}
	}
	r, ok := f.repos[s.URL]
	if err := f.errs[s.URL]; err != nil {
		return "", "", err
	}
	if !ok {
		var err error
		if r, err = gitx.Open(f.CacheDir, s); err == nil {
			host := ""
			if u, perr := url.Parse(s.URL); perr == nil {
				host = u.Hostname()
			}
			err = r.Fetch(f.Ctx, gitx.AuthEnv(host, f.Getenv, f.GhToken))
		}
		if err != nil {
			f.errs[s.URL] = err
			return "", "", err
		}
		f.repos[s.URL] = r
	}
	commit, err := r.Resolve(s.Ref)
	if err != nil {
		return "", "", err
	}
	tree, err := r.TreeSHA(commit, s.Path)
	return commit, tree, err
}

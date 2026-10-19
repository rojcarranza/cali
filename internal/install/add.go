// Package install puts skills from git sources into agent target folders and
// records them in cali.yaml and cali.lock.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rojcarranza/cali/internal/dirs"
	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// maxScanDepth is how deep add looks for SKILL.md when the spec has no //path.
const maxScanDepth = 4

var defaultTargets = []string{"claude", "copilot"}

// Deps are the outside-world inputs of an install.
type Deps struct {
	Dirs    dirs.Dirs
	Stdout  io.Writer // progress lines; nil discards them
	Getenv  func(string) string
	GhToken func() (string, error)
	// AllowFileScheme admits file:// sources (local test fixtures).
	AllowFileScheme bool
	// Rename moves a staged folder into place; nil means os.Rename.
	Rename func(oldpath, newpath string) error
	// Cwd is where project scope is resolved from; empty means os.Getwd.
	Cwd string
	// Global selects the global scope for Sync and Remove.
	Global bool
	// Manifest is --manifest: the manifest file, relative to Cwd; the lock sits
	// next to it and, in project scope, its dir is the project root. Empty means the default.
	Manifest string
}

// AddRequest is one `cali add` invocation.
type AddRequest struct {
	Spec    string
	Skill   string   // pick this skill when the spec has no //path
	All     bool     // kept for --all; a spec without //path or Skill installs every skill anyway
	Global  bool     // global scope instead of project scope
	Targets []string // --target; empty means the manifest's targets
	Adopt   bool     // take over an existing differing folder as is
}

// Installed describes one skill added by Add.
type Installed struct {
	Name, Path, Commit, Tree string
	Dirs                     []string // folders written
	Adopted                  []string // existing folders taken over unchanged
}

// Result is what Add did.
type Result struct {
	Manifest string // path of the manifest written
	Created  bool   // the manifest did not exist before
	Skills   []Installed
}

type scope struct {
	scope              targets.Scope
	root               string // project root; empty for global
	manifest, lockPath string
}

// resolveScope finds the manifest and lock paths. Project scope is rooted at
// the nearest ancestor of the cwd holding cali.yaml, else the nearest one
// holding .git, else the cwd. Global scope lives in the config dir.
// d.Manifest overrides both: the lock sits next to it and, in project scope,
// its dir is the root; global targets stay under home.
func resolveScope(d Deps, global bool) (scope, error) {
	if d.Manifest == "" && global {
		return scope{
			scope:    targets.Global,
			manifest: filepath.Join(d.Dirs.Config, "cali.yaml"),
			lockPath: filepath.Join(d.Dirs.Config, "cali.lock"),
		}, nil
	}
	cwd := d.Cwd
	if cwd == "" {
		wd, err := os.Getwd()
		if err != nil {
			return scope{}, err
		}
		cwd = wd
	}
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return scope{}, err
	}
	if d.Manifest != "" {
		mf := d.Manifest
		if !filepath.IsAbs(mf) {
			mf = filepath.Join(cwd, mf)
		}
		mf = filepath.Clean(mf)
		if !global && sameDir(filepath.Dir(mf), d.Dirs.Home) {
			return scope{}, fmt.Errorf("%s: a project manifest cannot sit in your home folder (its skills dirs are the global ones); use -g", mf)
		}
		sc := scope{scope: targets.Project, root: filepath.Dir(mf), manifest: mf, lockPath: filepath.Join(filepath.Dir(mf), "cali.lock")}
		if global {
			sc.scope, sc.root = targets.Global, ""
		}
		return sc, nil
	}
	root := findRoot(cwd, d.Dirs.Home)
	if sameDir(root, d.Dirs.Home) {
		return scope{}, fmt.Errorf("%s: a project manifest cannot sit in your home folder (its skills dirs are the global ones); use -g", filepath.Join(root, "cali.yaml"))
	}
	return scope{
		scope:    targets.Project,
		root:     root,
		manifest: filepath.Join(root, "cali.yaml"),
		lockPath: filepath.Join(root, "cali.lock"),
	}, nil
}

// sameDir compares two paths after symlink resolution, so HOME and the cwd
// agree when they spell one folder differently; a path that cannot be
// resolved (not created yet) falls back to its cleaned form.
func sameDir(a, b string) bool {
	return resolve(a) == resolve(b)
}

func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// findRoot walks up from cwd: the first dir holding cali.yaml or .git is the
// root. The walk stops before home and never checks home itself (a dotfiles
// ~/.git or a ~/cali.yaml must not make home, whose .claude/skills is the
// global dir, the project); with no match the root is cwd.
func findRoot(cwd, home string) string {
	for dir := cwd; !sameDir(dir, home); {
		for _, marker := range []string{"cali.yaml", ".git"} {
			if _, err := os.Lstat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

// noSymlinks refuses a symlink at any existing path component below root
// down to and including dir. Used in project scope, where a cloned repo could
// ship .claude as a link to somewhere sensitive.
func noSymlinks(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return err
	}
	cur := root
	for _, seg := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, seg)
		fi, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; Cali does not write through symlinks in a project", cur)
		}
	}
	return nil
}

// Init creates an empty manifest (version 1, targets claude and copilot) for
// the scope and returns its path. It refuses if the manifest exists.
func Init(d Deps, global bool) (string, error) {
	sc, err := resolveScope(d, global)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(sc.manifest); err == nil {
		return "", fmt.Errorf("%s already exists", sc.manifest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(sc.manifest), 0o755); err != nil {
		return "", err
	}
	m := manifest.Manifest{Version: 1, Targets: slices.Clone(defaultTargets), Skills: map[string]manifest.Skill{}}
	if err := manifest.WriteWith(sc.manifest, m, source.Options{AllowFile: d.AllowFileScheme}); err != nil {
		return "", err
	}
	return sc.manifest, nil
}

// plan is one skill ready to be written.
type plan struct {
	name, path, tree, source string
	tree0                    skillfs.Tree
	skillTargets             []string // the entry's own targets (nil = manifest default)
	write, adopt             []string // final folders to write, folders to adopt
}

// Add fetches req.Spec, installs the chosen skill(s) into every target folder
// and then writes the lock and manifest. Nothing is recorded unless every
// folder was written; folders written by a failed Add are removed again, and
// a folder that existed before is never changed.
func Add(ctx context.Context, d Deps, req AddRequest) (Result, error) {
	opts := source.Options{AllowFile: d.AllowFileScheme}
	out := d.Stdout
	if out == nil {
		out = io.Discard
	}
	if req.Skill != "" && req.All {
		return Result{}, errors.New("--skill and --all cannot be used together")
	}
	spec, err := source.ParseWith(req.Spec, opts)
	if err != nil {
		return Result{}, err
	}
	if spec.Path != "" && (req.Skill != "" || req.All) {
		return Result{}, errors.New("--skill and --all choose skills in a whole repo; drop them or the //path")
	}
	var reqTargets []string
	for _, t := range req.Targets {
		if _, err := targets.Dir(t, targets.Project, "", ""); err != nil {
			return Result{}, err
		}
		if !slices.Contains(reqTargets, t) {
			reqTargets = append(reqTargets, t)
		}
	}

	d.Global = req.Global // openFetch finds a same-repo source through the scope
	sc, err := resolveScope(d, req.Global)
	if err != nil {
		return Result{}, err
	}
	if err := checkScopeFiles(sc); err != nil {
		return Result{}, err
	}
	m, err := manifest.ReadWith(sc.manifest, opts)
	created := false
	switch {
	case errors.Is(err, os.ErrNotExist):
		created = true
		m = manifest.Manifest{Version: 1, Targets: slices.Clone(defaultTargets), Skills: map[string]manifest.Skill{}}
		if len(reqTargets) > 0 {
			m.Targets = reqTargets
		}
	case err != nil:
		return Result{}, err
	}
	l, err := lock.Read(sc.lockPath)
	if err != nil {
		return Result{}, err
	}

	repo, err := openFetch(ctx, d, spec)
	if err != nil {
		return Result{}, err
	}
	commit, err := repo.Resolve(spec.Ref)
	if err != nil {
		return Result{}, err
	}
	paths, err := pickPaths(repo, commit, spec, req)
	if err != nil {
		return Result{}, err
	}

	// Plan everything before writing anything.
	var plans []*plan
	seen := map[string]string{}
	for _, p := range paths {
		pl, err := planSkill(d, sc, m, repo, commit, spec, req, reqTargets, p, opts)
		if err != nil {
			return Result{}, err
		}
		if prev, dup := seen[pl.name]; dup {
			return Result{}, fmt.Errorf("two skills would be installed as %q: %s and %s", pl.name, show(prev), show(p))
		}
		seen[pl.name] = p
		plans = append(plans, pl)
	}

	for _, pl := range plans {
		if err := store.Put(d.Dirs.Data, pl.tree, pl.tree0); err != nil {
			return Result{}, fmt.Errorf("store pristine copy of %s: %w", pl.name, err)
		}
	}

	rename := d.Rename
	if rename == nil {
		rename = os.Rename
	}
	var made []string
	rollback := func() {
		for _, dir := range made {
			_ = os.RemoveAll(dir) // created by this Add; best effort
		}
	}
	for _, pl := range plans {
		for _, final := range pl.write {
			if err := place(final, pl.tree0, rename); err != nil {
				rollback()
				return Result{}, err
			}
			made = append(made, final)
		}
	}

	oldSkills := maps.Clone(m.Skills)
	res := Result{Manifest: sc.manifest, Created: created}
	for _, pl := range plans {
		m.Skills[pl.name] = manifest.Skill{Source: pl.source, Ref: spec.Ref, Targets: pl.skillTargets}
		l.Skills[pl.name] = lock.Entry{Source: spec.URL, Path: pl.path, Ref: spec.Ref, Commit: LockCommit(repo, commit), Tree: pl.tree}
		res.Skills = append(res.Skills, Installed{
			Name: pl.name, Path: pl.path, Commit: commit, Tree: pl.tree,
			Dirs: pl.write, Adopted: pl.adopt,
		})
	}
	if err := os.MkdirAll(filepath.Dir(sc.manifest), 0o755); err != nil {
		rollback()
		return Result{}, err
	}
	if err := manifest.WriteWith(sc.manifest, m, opts); err != nil {
		rollback()
		return Result{}, err
	}
	if err := lock.Write(sc.lockPath, l); err != nil {
		rollback()
		if created {
			_ = os.Remove(sc.manifest)
		} else {
			m.Skills = oldSkills
			_ = manifest.WriteWith(sc.manifest, m, opts) // best effort restore
		}
		return Result{}, err
	}

	if created {
		_, _ = fmt.Fprintf(out, "created %s\n", sc.manifest)
	}
	for _, s := range res.Skills {
		for _, dir := range s.Dirs {
			_, _ = fmt.Fprintf(out, "installed %s (%s) in %s\n", s.Name, s.Commit[:7], dir)
		}
		for _, dir := range s.Adopted {
			_, _ = fmt.Fprintf(out, "adopted %s (%s) in %s\n", s.Name, s.Commit[:7], dir)
		}
	}
	return res, nil
}

func planSkill(d Deps, sc scope, m manifest.Manifest, repo *gitx.Repo, commit string, spec source.Spec,
	req AddRequest, reqTargets []string, p string, opts source.Options) (*plan, error) {
	name := installName(spec, p)
	if !manifest.SkillNameValid(name) {
		return nil, fmt.Errorf("skill folder %q is not a valid install name: want lowercase letters, digits and hyphens, 1-64 characters, not a Windows device name", name)
	}
	if prev, ok := m.Skills[name]; ok {
		ps, perr := source.ParseWith(prev.Source, opts)
		if perr == nil && ps.URL == spec.URL && strings.Trim(ps.Path, "/") == p {
			return nil, fmt.Errorf("skill %q is already in %s", name, sc.manifest)
		}
		return nil, fmt.Errorf("name %q already installed from %s; remove it first", name, prev.Source)
	}
	tree, t, err := exportSkill(repo, commit, p, name)
	if err != nil {
		return nil, err
	}

	pl := &plan{name: name, path: p, tree: tree, tree0: t, source: manifestSource(req.Spec, spec, p)}
	if len(reqTargets) > 0 && !slices.Equal(reqTargets, m.Targets) {
		pl.skillTargets = reqTargets
	}
	eff := pl.skillTargets
	if len(eff) == 0 {
		eff = m.Targets
	}
	if len(eff) == 0 {
		return nil, fmt.Errorf("no targets: set targets in %s or pass --target", sc.manifest)
	}
	for _, tg := range eff {
		dir, err := targets.Dir(tg, sc.scope, sc.root, d.Dirs.Home)
		if err != nil {
			return nil, err
		}
		if sc.scope == targets.Project {
			if err := noSymlinks(sc.root, dir); err != nil {
				return nil, err
			}
		}
		final := filepath.Join(dir, name)
		adopt, err := existing(final, t, req.Adopt)
		if err != nil {
			return nil, err
		}
		if adopt {
			pl.adopt = append(pl.adopt, final)
		} else {
			pl.write = append(pl.write, final)
		}
	}
	return pl, nil
}

// exportSkill reads the skill folder p at commit and checks that its
// SKILL.md frontmatter names it name.
func exportSkill(repo *gitx.Repo, commit, p, name string) (string, skillfs.Tree, error) {
	tree, err := repo.TreeSHA(commit, p)
	if err != nil {
		return "", nil, err
	}
	t, err := exportTree(repo, tree, name)
	return tree, t, err
}

// exportTree reads the skill folder with tree SHA tree and checks that its
// SKILL.md frontmatter names it name.
func exportTree(repo *gitx.Repo, tree, name string) (skillfs.Tree, error) {
	t, err := repo.Export(tree)
	if err != nil {
		return nil, fmt.Errorf("skill %q: %w", name, err)
	}
	var md []byte
	for _, f := range t {
		if f.Path == "SKILL.md" {
			md = f.Data
		}
	}
	fm, err := skillfs.ParseFrontmatter(md)
	if err != nil {
		return nil, skillfs.Invalid(fmt.Errorf("skill %q: %w", name, err))
	}
	if fm.Name != name {
		return nil, skillfs.Invalid(fmt.Errorf("skill folder %q has name %q in SKILL.md; they must match (Copilot skips a skill whose name differs from its folder)", name, fm.Name))
	}
	return t, nil
}

// checkScopeFiles refuses a symlinked manifest or lock in project scope.
func checkScopeFiles(sc scope) error {
	if sc.scope != targets.Project {
		return nil
	}
	for _, p := range []string{sc.manifest, sc.lockPath} {
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink; Cali does not write through symlinks in a project", p)
		}
	}
	return nil
}

// openFetch opens the cache mirror for spec and fetches it. A same-repo
// source opens the repository that holds the scope's manifest instead.
func openFetch(ctx context.Context, d Deps, spec source.Spec) (*gitx.Repo, error) {
	if spec.URL == source.Here {
		sc, err := resolveScope(d, d.Global)
		if err != nil {
			return nil, err
		}
		return gitx.OpenLocal(filepath.Dir(sc.manifest))
	}
	repo, err := gitx.Open(d.Dirs.Cache, spec)
	if err != nil {
		return nil, err
	}
	getenv := d.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if err := repo.Fetch(ctx, gitx.AuthEnv(host(spec.URL), getenv, d.GhToken)); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", spec.URL, err)
	}
	return repo, nil
}

// existing reports whether final already exists as an unmanaged folder that
// will be adopted. It errors if final exists and must not be touched.
func existing(final string, upstream skillfs.Tree, adoptFlag bool) (bool, error) {
	fi, err := os.Lstat(final)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	case fi.Mode()&os.ModeSymlink != 0:
		return false, fmt.Errorf("%s is a symlink not managed by Cali; remove it to install here", final)
	case !fi.IsDir():
		return false, fmt.Errorf("%s exists and is not a directory", final)
	}
	have, err := skillfs.ReadDir(final)
	if err != nil {
		return false, fmt.Errorf("%s exists, is not managed by Cali and cannot be compared: %w", final, err)
	}
	if skillfs.Equal(have, upstream) || adoptFlag {
		return true, nil
	}
	return false, fmt.Errorf("%s exists, is not managed by Cali and differs from upstream; pass --adopt to take it over as is", final)
}

// Stage writes t to a fresh .cali-staging-* dir in parent and returns it. On
// error nothing is left behind.
func Stage(parent string, t skillfs.Tree) (string, error) {
	staging, err := os.MkdirTemp(parent, ".cali-staging-*")
	if err != nil {
		return "", err
	}
	// MkdirTemp creates 0700; the installed folder must be readable like any other.
	if err := os.Chmod(staging, 0o755); err != nil {
		_ = os.RemoveAll(staging)
		return "", err
	}
	if err := t.WriteTo(staging); err != nil {
		_ = os.RemoveAll(staging)
		return "", fmt.Errorf("write %s: %w", staging, err)
	}
	return staging, nil
}

// MoveAside renames dir to a fresh, unused .cali-old-* path beside it and
// returns that path.
func MoveAside(dir string, rename func(string, string) error) (string, error) {
	old, err := os.MkdirTemp(filepath.Dir(dir), ".cali-old-*")
	if err == nil {
		err = os.Remove(old) // reserve a unique name, then free it for the rename
	}
	if err == nil {
		err = rename(dir, old)
	}
	if err != nil {
		return "", fmt.Errorf("move %s aside: %w", dir, err)
	}
	return old, nil
}

// place writes t to a fresh staging dir next to final and renames it to
// final. Only that staging dir is ever removed.
func place(final string, t skillfs.Tree, rename func(string, string) error) error {
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return err
	}
	staging, err := Stage(filepath.Dir(final), t)
	if err != nil {
		return err
	}
	// Re-check right before the swap: something may have created final since planning.
	if _, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
		_ = os.RemoveAll(staging)
		if err != nil {
			return err
		}
		return fmt.Errorf("%s appeared while installing; not overwriting it", final)
	}
	if err := rename(staging, final); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("move %s into place: %w", final, err)
	}
	return nil
}

// pickPaths returns the skill folders to install: the spec's //path, or
// those found by scanning the commit for SKILL.md.
func pickPaths(repo *gitx.Repo, commit string, spec source.Spec, req AddRequest) ([]string, error) {
	if spec.Path != "" {
		return []string{strings.Trim(spec.Path, "/")}, nil
	}
	found, err := repo.ListSkillDirs(commit, maxScanDepth)
	if err != nil {
		return nil, err
	}
	scanned := len(found)
	if repo.Local() {
		// Installed copies a project commits (<root>/.claude/skills, .github/skills) are not sources.
		found = slices.DeleteFunc(found, func(p string) bool {
			p = "/" + p + "/"
			return strings.Contains(p, "/.claude/skills/") || strings.Contains(p, "/.github/skills/")
		})
	}
	if len(found) == 0 && scanned > 0 {
		return nil, fmt.Errorf("only installed copies (.claude/skills, .github/skills) found in %s; name the skill folder with .//<path>", spec.URL)
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no SKILL.md found in %s (searched %d levels deep)", spec.URL, maxScanDepth)
	}
	list := make([]string, len(found))
	for i, p := range found {
		list[i] = show(p)
	}
	if req.Skill == "" { // a whole repo installs every skill, with or without --all
		return found, nil
	}
	var hits []string
	for _, p := range found {
		if installName(spec, p) == req.Skill {
			hits = append(hits, p)
		}
	}
	switch len(hits) {
	case 0:
		return nil, fmt.Errorf("no skill named %q in %s; found: %s", req.Skill, spec.URL, strings.Join(list, ", "))
	case 1:
		return hits, nil
	default:
		for i, h := range hits {
			hits[i] = show(h)
		}
		return nil, fmt.Errorf("several skills named %q in %s; use a //path to pick one: %s", req.Skill, spec.URL, strings.Join(hits, ", "))
	}
}

func show(p string) string {
	if p == "" {
		return "(repo root)"
	}
	return fmt.Sprintf("%q", p) // repo-controlled: quoted so control characters cannot reach the terminal
}

// installName is the skill folder's base name, or the repo name for a skill
// at the repository root.
func installName(spec source.Spec, p string) string {
	if p != "" {
		return path.Base(p)
	}
	return path.Base(strings.TrimRight(spec.URL, "/"))
}

// manifestSource is the source string stored in cali.yaml: the github: form
// if the user typed it, else the canonical URL, plus //path. The ref is
// stored separately.
func manifestSource(raw string, spec source.Spec, p string) string {
	s := spec.URL
	if strings.HasPrefix(raw, "github:") {
		s = "github:" + strings.TrimPrefix(spec.URL, "https://github.com/")
	}
	if p != "" {
		s += "//" + p
	}
	return s
}

// LockCommit is the commit a lock entry records for a skill read from repo at
// commit: none for a local repository, because the commit that will hold the
// lock does not exist yet and a squash merge drops the branch's commits. The
// tree alone identifies the skill.
func LockCommit(repo *gitx.Repo, commit string) string {
	if repo.Local() {
		return ""
	}
	return commit
}

func host(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Location is where a scope keeps its files.
type Location struct {
	Scope              targets.Scope
	Root               string // project root; empty for global
	Manifest, LockPath string
}

// Locate resolves the manifest, lock and project root for the scope without touching disk.
func Locate(d Deps, global bool) (Location, error) {
	sc, err := resolveScope(d, global)
	return Location{sc.scope, sc.root, sc.manifest, sc.lockPath}, err
}

// OpenFetch opens the cache mirror for spec and fetches it, with the auth of d.
func OpenFetch(ctx context.Context, d Deps, spec source.Spec) (*gitx.Repo, error) {
	return openFetch(ctx, d, spec)
}

// NoSymlinks refuses a symlink at any existing path component below root
// down to and including dir.
func NoSymlinks(root, dir string) error { return noSymlinks(root, dir) }

// ExportTree reads the skill folder with tree SHA tree (from TreeSHA);
// SKILL.md must name the skill name.
func ExportTree(repo *gitx.Repo, tree, name string) (skillfs.Tree, error) {
	return exportTree(repo, tree, name)
}

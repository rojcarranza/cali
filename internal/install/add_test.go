package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/dirs"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
)

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// isolate keeps the user's git config out of the test.
func isolate(t *testing.T) {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, rel, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillMD(name string) string {
	return "---\nname: " + name + "\ndescription: The " + name + " skill.\n---\n# " + name + "\n"
}

// repo builds a git repo named name holding files (path -> content) and
// returns its directory.
func repo(t *testing.T, name string, files map[string]string) string {
	t.Helper()
	isolate(t)
	dir := filepath.Join(t.TempDir(), name)
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "init", "-q", "-b", "main")
	for p, c := range files {
		write(t, dir, p, c)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	return dir
}

func monorepo(t *testing.T) string {
	return repo(t, "skills-repo", map[string]string{
		"skills/x/SKILL.md":       skillMD("x"),
		"skills/x/scripts/run.sh": "echo x\n",
		"skills/y/SKILL.md":       skillMD("y"),
		"README.md":               "not a skill\n",
	})
}

func fileURL(dir string) string { return "file://" + filepath.ToSlash(dir) }

type env struct {
	deps    Deps
	project string
}

func newEnv(t *testing.T) env {
	t.Helper()
	base := t.TempDir()
	project := filepath.Join(base, "project")
	for _, d := range []string{project, filepath.Join(base, "home")} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return env{project: project, deps: Deps{
		Dirs: dirs.Dirs{
			Config: filepath.Join(base, "config"),
			Data:   filepath.Join(base, "data"),
			Cache:  filepath.Join(base, "cache"),
			Home:   filepath.Join(base, "home"),
		},
		Getenv:          func(string) string { return "" },
		GhToken:         func() (string, error) { return "", errors.New("no gh") },
		AllowFileScheme: true,
		Cwd:             project,
	}}
}

func readManifest(t *testing.T, p string) manifest.Manifest {
	t.Helper()
	m, err := manifest.ReadWith(p, source.Options{AllowFile: true})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func readTree(t *testing.T, dir string) skillfs.Tree {
	t.Helper()
	tr, err := skillfs.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func TestAddInstallsSkillAndWritesLock(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	res, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(e.project, ".claude", "skills", "x")
	if !skillfs.Equal(readTree(t, installed), readTree(t, filepath.Join(r, "skills", "x"))) {
		t.Fatal("installed folder differs from the committed tree")
	}
	if exists(filepath.Join(e.project, ".github", "skills", "x")) {
		t.Fatal("copilot target must not be written for --target claude")
	}
	if exists(installed + ".cali-new") {
		t.Fatal("staging dir left behind")
	}

	l, err := lock.Read(filepath.Join(e.project, "cali.lock"))
	if err != nil {
		t.Fatal(err)
	}
	ent, ok := l.Skills["x"]
	if !ok || !sha40.MatchString(ent.Commit) || !sha40.MatchString(ent.Tree) {
		t.Fatalf("lock entry = %+v", ent)
	}
	if ent.Source != fileURL(r) || ent.Path != "skills/x" {
		t.Fatalf("lock entry = %+v", ent)
	}
	if ent.Commit != git(t, r, "rev-parse", "HEAD") || ent.Tree != git(t, r, "rev-parse", "HEAD:skills/x") {
		t.Fatalf("lock commit/tree wrong: %+v", ent)
	}

	m := readManifest(t, filepath.Join(e.project, "cali.yaml"))
	if m.Version != 1 || strings.Join(m.Targets, ",") != "claude" {
		t.Fatalf("manifest = %+v", m)
	}
	if s := m.Skills["x"]; s.Source != fileURL(r)+"//skills/x" {
		t.Fatalf("manifest entry = %+v", s)
	}

	pristine, ok, err := store.Get(e.deps.Dirs.Data, ent.Tree)
	if err != nil || !ok || !skillfs.Equal(pristine, readTree(t, installed)) {
		t.Fatalf("pristine missing or different: ok=%v err=%v", ok, err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "x" {
		t.Fatalf("result = %+v", res)
	}
}

func TestAddDefaultTargetsWritesBoth(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".claude/skills/x", ".github/skills/x"} {
		if !exists(filepath.Join(e.project, filepath.FromSlash(d), "SKILL.md")) {
			t.Errorf("%s not installed", d)
		}
	}
	m := readManifest(t, filepath.Join(e.project, "cali.yaml"))
	if strings.Join(m.Targets, ",") != "claude,copilot" {
		t.Fatalf("targets = %v", m.Targets)
	}
}

func TestAddRejectsNameMismatch(t *testing.T) {
	r := repo(t, "r", map[string]string{"skills/x/SKILL.md": skillMD("other")})
	e := newEnv(t)
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"})
	if err == nil || !strings.Contains(err.Error(), `"x"`) || !strings.Contains(err.Error(), `"other"`) {
		t.Fatalf("err = %v, want both names", err)
	}
	if exists(filepath.Join(e.project, ".claude")) || exists(filepath.Join(e.project, "cali.yaml")) {
		t.Fatal("nothing may be written on a name mismatch")
	}
}

func TestAddRejectsInvalidInstallName(t *testing.T) {
	r := repo(t, "r", map[string]string{"skills/Bad_Name/SKILL.md": skillMD("Bad_Name")})
	e := newEnv(t)
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/Bad_Name"})
	if err == nil || !strings.Contains(err.Error(), "Bad_Name") {
		t.Fatalf("err = %v", err)
	}
}

func TestAddRefusesUnmanagedFolder(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	target := filepath.Join(e.project, ".claude", "skills", "x")
	write(t, target, "SKILL.md", "my own edits\n")
	req := AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}

	_, err := Add(context.Background(), e.deps, req)
	if err == nil || !strings.Contains(err.Error(), "--adopt") {
		t.Fatalf("err = %v, want unmanaged error mentioning --adopt", err)
	}
	if b, _ := os.ReadFile(filepath.Join(target, "SKILL.md")); string(b) != "my own edits\n" {
		t.Fatal("unmanaged folder was changed")
	}
	if exists(filepath.Join(e.project, "cali.yaml")) || exists(filepath.Join(e.project, "cali.lock")) {
		t.Fatal("manifest or lock written after refusal")
	}

	req.Adopt = true
	if _, err := Add(context.Background(), e.deps, req); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(target, "SKILL.md")); string(b) != "my own edits\n" {
		t.Fatal("--adopt must keep the folder as is")
	}
	if _, ok := readManifest(t, filepath.Join(e.project, "cali.yaml")).Skills["x"]; !ok {
		t.Fatal("adopted skill not in manifest")
	}
}

func TestAddAdoptsIdenticalFolder(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	target := filepath.Join(e.project, ".claude", "skills", "x")
	if err := readTree(t, filepath.Join(r, "skills", "x")).WriteTo(target); err != nil {
		t.Fatal(err)
	}
	res, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 || len(res.Skills[0].Adopted) != 1 {
		t.Fatalf("result = %+v, want one adopted dir", res)
	}
	if _, ok := readManifest(t, filepath.Join(e.project, "cali.yaml")).Skills["x"]; !ok {
		t.Fatal("adopted skill not in manifest")
	}
}

func TestAddNameClashWithOtherSource(t *testing.T) {
	r1 := monorepo(t)
	r2 := repo(t, "other", map[string]string{"x/SKILL.md": skillMD("x")})
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r1) + "//skills/x"}); err != nil {
		t.Fatal(err)
	}
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r2) + "//x"})
	if err == nil || !strings.Contains(err.Error(), "remove it first") || strings.Contains(err.Error(), "--replace") {
		t.Fatalf("err = %v, want remove hint without the nonexistent --replace", err)
	}
}

func TestAddDiscovery(t *testing.T) {
	r := monorepo(t)

	e := newEnv(t)
	res, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r)})
	if err != nil || len(res.Skills) != 2 {
		t.Fatalf("whole repo: res = %+v, err = %v; want both skills installed", res, err)
	}

	e = newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r), Skill: "y"}); err != nil {
		t.Fatal(err)
	}
	m := readManifest(t, filepath.Join(e.project, "cali.yaml"))
	if len(m.Skills) != 1 || m.Skills["y"].Source != fileURL(r)+"//skills/y" {
		t.Fatalf("manifest = %+v", m)
	}

	e = newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r), Skill: "nope"}); err == nil || !strings.Contains(err.Error(), "skills/x") {
		t.Fatalf("err = %v, want candidates listed", err)
	}

	e = newEnv(t)
	res, err = Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r), All: true})
	if err != nil || len(res.Skills) != 2 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	l, err := lock.Read(filepath.Join(e.project, "cali.lock"))
	if err != nil || len(l.Skills) != 2 {
		t.Fatalf("lock = %+v, err = %v", l, err)
	}

	e = newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", All: true}); err == nil {
		t.Fatal("want error for --all with a //path")
	}
}

func TestAddRootSkillUsesRepoName(t *testing.T) {
	r := repo(t, "solo", map[string]string{"SKILL.md": skillMD("solo"), "ref.md": "r\n"})
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r), Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(e.project, ".claude", "skills", "solo", "ref.md")) {
		t.Fatal("root skill not installed under the repo name")
	}
	l, _ := lock.Read(filepath.Join(e.project, "cali.lock"))
	if ent := l.Skills["solo"]; ent.Path != "" || !sha40.MatchString(ent.Tree) {
		t.Fatalf("lock entry = %+v", ent)
	}
}

func TestAddRollsBackWhenATargetFails(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	calls := 0
	e.deps.Rename = func(o, n string) error {
		calls++
		if calls == 2 {
			return errors.New("injected rename failure")
		}
		return os.Rename(o, n)
	}
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"})
	if err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("err = %v", err)
	}
	for _, d := range []string{".claude/skills", ".github/skills"} {
		entries, _ := os.ReadDir(filepath.Join(e.project, filepath.FromSlash(d)))
		for _, en := range entries {
			t.Errorf("%s/%s left behind after failure", d, en.Name())
		}
	}
	for _, p := range []string{"cali.yaml", "cali.lock"} {
		if exists(filepath.Join(e.project, p)) {
			t.Errorf("%s written after failure", p)
		}
	}
}

func TestAddLeavesForeignStagingNameAlone(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	other := filepath.Join(e.project, ".claude", "skills", "x.cali-new")
	write(t, other, "keep", "not ours\n")
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(other, "keep")) || exists(filepath.Join(e.project, ".claude", "skills", "x", "keep")) {
		t.Fatal("x.cali-new must be left untouched and not used")
	}
	fi, err := os.Stat(filepath.Join(e.project, ".claude", "skills", "x"))
	if err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o755) {
		t.Fatalf("installed dir mode = %v, %v; want 0755", fi.Mode().Perm(), err)
	}
}

func TestPlaceRefusesTargetThatAppeared(t *testing.T) {
	skills := t.TempDir()
	final := filepath.Join(skills, "x")
	tr := skillfs.Tree{{Path: "SKILL.md", Data: []byte(skillMD("x"))}}
	rename := func(o, n string) error {
		t.Fatal("rename must not be called")
		return nil
	}
	// Simulate another process creating the folder after planning.
	write(t, final, "theirs", "t\n")
	err := place(final, tr, rename)
	if err == nil {
		t.Fatal("want error")
	}
	entries, _ := os.ReadDir(skills)
	if len(entries) != 1 || !exists(filepath.Join(final, "theirs")) {
		t.Fatalf("staging left behind or target changed: %v", entries)
	}
}

func TestAddGlobalScope(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Global: true}); err != nil {
		t.Fatal(err)
	}
	home := e.deps.Dirs.Home
	for _, d := range []string{".claude/skills/x", ".copilot/skills/x"} {
		if !exists(filepath.Join(home, filepath.FromSlash(d), "SKILL.md")) {
			t.Errorf("%s not installed in home", d)
		}
	}
	if _, ok := readManifest(t, filepath.Join(e.deps.Dirs.Config, "cali.yaml")).Skills["x"]; !ok {
		t.Fatal("global manifest missing entry")
	}
	if exists(filepath.Join(e.project, "cali.yaml")) || exists(filepath.Join(e.project, ".claude")) {
		t.Fatal("global add wrote into the project")
	}
}

func TestProjectRootResolution(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if err := os.MkdirAll(filepath.Join(e.project, "sub", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(e.project, "sub", "deep")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	e.deps.Cwd = cwd
	// A cali.yaml above the git root is ignored: the walk stops at .git.
	if err := os.WriteFile(filepath.Join(e.project, "cali.yaml"), []byte("version: 1\ntargets: [copilot]\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(e.deps, false); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(e.project, "sub", "cali.yaml")) {
		t.Fatal("init did not use the git root")
	}
	if _, err := Init(e.deps, false); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Fatalf("second init err = %v", err)
	}
	if err := os.Remove(filepath.Join(e.project, "sub", "cali.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(e.project, "sub", ".claude", "skills", "x", "SKILL.md")) || exists(filepath.Join(e.project, ".github")) {
		t.Fatal("add did not install into the git root")
	}
	if !exists(filepath.Join(e.project, "sub", "cali.lock")) || exists(filepath.Join(e.project, "cali.lock")) {
		t.Fatal("lock not at the git root")
	}

	// A cali.yaml between cwd and the git root wins.
	if err := os.WriteFile(filepath.Join(cwd, "cali.yaml"), []byte("version: 1\ntargets: [claude]\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := Init(e.deps, false)
	if err == nil || !strings.Contains(err.Error(), filepath.Join(cwd, "cali.yaml")) {
		t.Fatalf("init = %s, %v; want refusal naming the nearer cali.yaml", p, err)
	}
}

func TestProjectRootStopsAtHome(t *testing.T) {
	e := newEnv(t)
	home := e.deps.Dirs.Home
	// A stray cali.yaml in HOME's parent must not capture projects in HOME.
	if err := os.WriteFile(filepath.Join(filepath.Dir(home), "cali.yaml"), []byte("version: 1\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(home, "work", "proj")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	e.deps.Cwd = cwd
	p, err := Init(e.deps, false)
	if err != nil || p != filepath.Join(cwd, "cali.yaml") {
		t.Fatalf("init = %s, %v; want cwd", p, err)
	}
}

func TestProjectRootNeverHome(t *testing.T) {
	e := newEnv(t)
	home := e.deps.Dirs.Home
	// A dotfiles repo (~/.git) and a ~/cali.yaml must not make HOME a project.
	if err := os.Mkdir(filepath.Join(home, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "cali.yaml"), []byte("version: 1\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(home, "work", "scratch")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	e.deps.Cwd = cwd
	sc, err := resolveScope(e.deps, false)
	if err != nil || sc.root != cwd {
		t.Fatalf("root = %s, %v; want cwd %s", sc.root, err, cwd)
	}
}

func TestProjectRootNeverHomeThroughSymlink(t *testing.T) {
	e := newEnv(t)
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Mkdir(filepath.Join(real, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	e.deps.Dirs.Home = link
	cwd := filepath.Join(real, "notes")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	e.deps.Cwd = cwd
	sc, err := resolveScope(e.deps, false)
	if err != nil || sc.root != cwd {
		t.Fatalf("root = %s, %v; want cwd %s", sc.root, err, cwd)
	}
	e.deps.Cwd = real
	if sc, err := resolveScope(e.deps, false); err == nil {
		t.Fatalf("cwd = home via the other spelling: root = %s, want refusal", sc.root)
	}
	e.deps.Cwd = cwd
	e.deps.Manifest = filepath.Join(real, "cali.yaml")
	if sc, err := resolveScope(e.deps, false); err == nil {
		t.Fatalf("--manifest in home via the other spelling: root = %s, want refusal", sc.root)
	}
}

func TestAddRefusesSymlinkInProjectTree(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(e.project, ".claude")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}})
	if err == nil || !strings.Contains(err.Error(), filepath.Join(e.project, ".claude")) {
		t.Fatalf("err = %v, want refusal naming .claude", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote through the symlink: %v", entries)
	}
	if exists(filepath.Join(e.project, "cali.yaml")) {
		t.Fatal("manifest written after refusal")
	}
}

func TestAddRefusesSymlinkedLockInProject(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	victim := filepath.Join(t.TempDir(), "victim")
	write(t, filepath.Dir(victim), "victim", "keep\n")
	if err := os.Symlink(victim, filepath.Join(e.project, "cali.lock")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"})
	if err == nil || !strings.Contains(err.Error(), "cali.lock") {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(victim); string(b) != "keep\n" || exists(filepath.Join(e.project, ".claude")) {
		t.Fatal("wrote after refusing the symlinked lock")
	}
}

func TestAddGlobalAllowsSymlinkedSkillsDir(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	dotfiles := t.TempDir()
	if err := os.MkdirAll(filepath.Join(e.deps.Dirs.Home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dotfiles, filepath.Join(e.deps.Dirs.Home, ".claude", "skills")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Global: true, Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(dotfiles, "x", "SKILL.md")) {
		t.Fatal("global install did not go through the symlinked skills dir")
	}
}

func TestInitWritesDefaultManifest(t *testing.T) {
	e := newEnv(t)
	p, err := Init(e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if p != filepath.Join(e.project, "cali.yaml") {
		t.Fatalf("path = %s", p)
	}
	m := readManifest(t, p)
	if m.Version != 1 || strings.Join(m.Targets, ",") != "claude,copilot" || len(m.Skills) != 0 {
		t.Fatalf("manifest = %+v", m)
	}
	g, err := Init(e.deps, true)
	if err != nil || g != filepath.Join(e.deps.Dirs.Config, "cali.yaml") {
		t.Fatalf("global init = %s, %v", g, err)
	}
}

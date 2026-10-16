package install

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
	if err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("err = %v, want --replace hint", err)
	}
}

func TestAddDiscovery(t *testing.T) {
	r := monorepo(t)

	e := newEnv(t)
	_, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r)})
	if err == nil || !strings.Contains(err.Error(), "skills/x") || !strings.Contains(err.Error(), "skills/y") {
		t.Fatalf("err = %v, want both candidates listed", err)
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
	res, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r), All: true})
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
	for _, p := range []string{
		".claude/skills/x", ".claude/skills/x.cali-new",
		".github/skills/x", ".github/skills/x.cali-new",
		"cali.yaml", "cali.lock",
	} {
		if exists(filepath.Join(e.project, filepath.FromSlash(p))) {
			t.Errorf("%s left behind after failure", p)
		}
	}
}

func TestAddReplacesStaleStagingDir(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	stale := filepath.Join(e.project, ".claude", "skills", "x.cali-new")
	write(t, stale, "junk", "left from a crash\n")
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	if exists(stale) || exists(filepath.Join(e.project, ".claude", "skills", "x", "junk")) {
		t.Fatal("stale staging content survived")
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
	// A cali.yaml in an ancestor wins over a nearer .git.
	if err := os.MkdirAll(filepath.Join(e.project, "sub", ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(e.project, "sub", "deep")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	e.deps.Cwd = cwd
	// No cali.yaml anywhere: the git root (sub) is used.
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
	if err := os.WriteFile(filepath.Join(e.project, "cali.yaml"), []byte("version: 1\ntargets: [copilot]\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(e.project, ".github", "skills", "x", "SKILL.md")) || exists(filepath.Join(e.project, ".claude")) {
		t.Fatal("add did not install into the cali.yaml root with its targets")
	}
	if exists(filepath.Join(e.project, "sub", "cali.yaml")) || !exists(filepath.Join(e.project, "cali.lock")) {
		t.Fatal("lock/manifest not at the cali.yaml root")
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

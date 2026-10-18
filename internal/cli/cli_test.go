package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/lock"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// setup isolates HOME, the cali dirs, git config and the cwd, allows file://
// sources, and returns a fixture repo URL and the project dir.
func setup(t *testing.T) (repoURL, project string) {
	t.Helper()
	base := t.TempDir()
	empty := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("USERPROFILE", filepath.Join(base, "home"))
	t.Setenv("CALI_CONFIG_DIR", filepath.Join(base, "config"))
	t.Setenv("CALI_DATA_DIR", filepath.Join(base, "data"))
	t.Setenv("CALI_CACHE_DIR", filepath.Join(base, "cache"))
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	r := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(r, "skills", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r, "skills", "x", "SKILL.md"), []byte("---\nname: x\ndescription: X.\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, r, "init", "-q", "-b", "main")
	gitRun(t, r, "add", ".")
	gitRun(t, r, "commit", "-q", "-m", "one")

	project = filepath.Join(base, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	old := allowFileScheme
	allowFileScheme = true
	t.Cleanup(func() { allowFileScheme = old })
	return "file://" + filepath.ToSlash(r), project
}

func run(args ...string) (code int, stdout, stderr string) {
	var out, errb bytes.Buffer
	code = Execute(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestAddCommandEndToEnd(t *testing.T) {
	repoURL, project := setup(t)

	if code, out, errs := run("init"); code != 0 || !strings.Contains(out, "cali.yaml") {
		t.Fatalf("init: code=%d out=%q err=%q", code, out, errs)
	}
	if code, _, errs := run("init"); code != 1 || !strings.Contains(errs, "exists") {
		t.Fatalf("second init: code=%d err=%q", code, errs)
	}

	code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude")
	if code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	if !strings.Contains(out, "x") {
		t.Fatalf("add output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "x", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	l, err := lock.Read(filepath.Join(project, "cali.lock"))
	if err != nil || l.Skills["x"].Commit == "" {
		t.Fatalf("lock = %+v, err = %v", l, err)
	}
}

func TestAddCommandGlobalAndErrors(t *testing.T) {
	repoURL, _ := setup(t)
	code, out, errs := run("add", "-g", repoURL)
	if code != 0 {
		t.Fatalf("add -g: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".copilot", "skills", "x", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if code, _, errs := run("add"); code != 1 || errs == "" {
		t.Fatalf("add without spec: code=%d err=%q", code, errs)
	}
	if code, _, errs := run("add", repoURL, "--target", "vim"); code != 1 || !strings.Contains(errs, "vim") {
		t.Fatalf("bad target: code=%d err=%q", code, errs)
	}
}

func TestFileSchemeRejectedByDefault(t *testing.T) {
	repoURL, _ := setup(t)
	allowFileScheme = false
	if code, _, errs := run("add", repoURL); code != 1 || !strings.Contains(errs, "file") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestVersion(t *testing.T) {
	if code, out, _ := run("--version"); code != 0 || !strings.Contains(out, "cali") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestStatusExitCode(t *testing.T) {
	repoURL, project := setup(t)
	repo := filepath.FromSlash(strings.TrimPrefix(repoURL, "file://"))
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	status := func(wantCode int, wantState string, args ...string) string {
		t.Helper()
		code, out, errs := run(append([]string{"status"}, args...)...)
		if code != wantCode || !strings.Contains(out, wantState) {
			t.Fatalf("status %v: code=%d out=%q err=%q", args, code, out, errs)
		}
		return out
	}
	out := status(0, "ok")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 || !strings.HasPrefix(lines[0], "SKILL") ||
		!strings.HasPrefix(lines[1], "x") || !strings.Contains(lines[1], "claude") {
		t.Fatalf("out = %q", out)
	}
	status(0, "ok", "--exit-code")

	// A newer commit outside skills/x is not outdated.
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "readme")
	status(0, "ok", "--exit-code")

	// One that touches skills/x is.
	if err := os.WriteFile(filepath.Join(repo, "skills", "x", "more.md"), []byte("more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "more")
	status(0, "outdated")
	status(1, "outdated", "--exit-code")
	status(0, "ok", "--offline", "--exit-code")

	// A local edit that also declares a missing dependency.
	p := filepath.Join(project, ".claude", "skills", "x", "SKILL.md")
	if err := os.WriteFile(p, []byte("---\nname: x\ndescription: X.\nmetadata:\n  cali-requires: \"dep\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = status(1, "modified", "--offline", "--exit-code")
	if !strings.HasSuffix(out, "WARN x requires dep (missing)\n") {
		t.Fatalf("out = %q", out)
	}

	code, out, errs := run("status", "--offline", "--json")
	var got struct {
		SchemaVersion int `json:"schema_version"`
		Rows          []struct {
			Skill, Target, Detail string
			States                []string
		} `json:"rows"`
		Warnings []struct{ Skill, Missing, Target string } `json:"warnings"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	if got.SchemaVersion != 1 || len(got.Rows) != 1 || strings.Join(got.Rows[0].States, ",") != "modified" ||
		len(got.Warnings) != 1 || got.Warnings[0].Missing != "dep" || got.Warnings[0].Target != "claude" {
		t.Fatalf("got %+v", got)
	}

	// A broken source is a warning, not a failure.
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	code, _, errs = run("status")
	if code != 0 || !strings.HasPrefix(errs, "warning: "+repoURL+": ") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestSyncAndRemoveCommands(t *testing.T) {
	repoURL, project := setup(t)
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	dir := filepath.Join(project, ".claude", "skills", "x")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run("sync", "--frozen"); code != 0 || out != "installed x (claude)\n" {
		t.Fatalf("sync: code=%d out=%q err=%q", code, out, errs)
	}
	if code, _, errs := run("remove", "nope"); code != 1 || !strings.Contains(errs, `"nope"`) {
		t.Fatalf("remove nope: code=%d err=%q", code, errs)
	}
	code, out, errs := run("remove", "x")
	if code != 0 || !strings.Contains(out, "backed up x (claude) to "+filepath.Join(os.Getenv("CALI_DATA_DIR"), "backups", "x")) {
		t.Fatalf("remove: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatal("folder not removed")
	}
}

func TestUpdateDryRunCommand(t *testing.T) {
	repoURL, project := setup(t)
	repo := filepath.FromSlash(strings.TrimPrefix(repoURL, "file://"))
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, _ := run("update"); code != 0 || out != "x: up to date\n0 skill(s) to update, 0 conflict(s)\n" {
		t.Fatalf("code=%d out=%q", code, out)
	}
	// Local and upstream rewrite the same body line: a conflict exits 1 with no error text.
	if err := os.WriteFile(filepath.Join(project, ".claude", "skills", "x", "SKILL.md"), []byte("---\nname: x\ndescription: X.\n---\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "skills", "x", "SKILL.md"), []byte("---\nname: x\ndescription: X.\n---\ntheirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "commit", "-q", "-am", "two")
	code, out, errs := run("update")
	if code != 1 || errs != "" || !strings.Contains(out, "cali update x --apply --resolve SKILL.md=") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	code, out, _ = run("update", "--json")
	if code != 1 || !json.Valid([]byte(out)) {
		t.Fatalf("json: code=%d out=%q", code, out)
	}
	for args, want := range map[string]string{
		"update x --apply":                   "nothing was written",
		"update --resolve SKILL.md=theirs":   "cali: --resolve needs exactly one skill name\n",
		"update x --resolve SKILL.md=both":   "cali: --resolve \"SKILL.md=both\": want <file>=ours|theirs|merge\n",
		"update nope":                        "is not in",
		"update --take-upstream --keep-mine": "cali: --take-upstream and --keep-mine cannot be used together\n",
		"update --apply --json":              "cali: --apply and --json cannot be used together\n",
	} {
		if code, _, errs := run(strings.Fields(args)...); code != 1 || !strings.Contains(errs, want) {
			t.Errorf("%s: code=%d err=%q", args, code, errs)
		}
	}
	// Not a TTY: the explicit choice applies without a prompt.
	code, out, errs = run("update", "x", "--apply", "--resolve", "SKILL.md=theirs")
	if code != 0 || !strings.HasPrefix(out, "x (claude) backup: ") || !strings.Contains(out, "\nupdated x ") {
		t.Fatalf("apply: code=%d out=%q err=%q", code, out, errs)
	}
	if b, err := os.ReadFile(filepath.Join(project, ".claude", "skills", "x", "SKILL.md")); err != nil || string(b) != "---\nname: x\ndescription: X.\n---\ntheirs\n" {
		t.Fatalf("SKILL.md = %q, %v", b, err)
	}
	if code, out, _ := run("update"); code != 0 || out != "x: up to date\n0 skill(s) to update, 0 conflict(s)\n" {
		t.Fatalf("after apply: code=%d out=%q", code, out)
	}
}

func TestGhTokenRunsOncePerCommand(t *testing.T) {
	setup(t)
	runs := 0
	orig := ghToken
	ghToken = func() (string, error) { runs++; return "tok", nil }
	t.Cleanup(func() { ghToken = orig })
	d, err := deps(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if tok, err := d.GhToken(); tok != "tok" || err != nil {
			t.Fatalf("token = %q, %v", tok, err)
		}
	}
	if runs != 1 {
		t.Fatalf("gh auth token ran %d times, want 1", runs)
	}
}

func TestManifestFlagGlobal(t *testing.T) {
	repoURL, project := setup(t)
	dot := filepath.Join(filepath.Dir(project), "dotfiles")
	mf := filepath.Join(dot, "cali.yaml")
	if code, out, errs := run("add", "-g", "--manifest", mf, repoURL, "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	for _, p := range []string{mf, filepath.Join(dot, "cali.lock"), filepath.Join(os.Getenv("HOME"), ".claude", "skills", "x", "SKILL.md")} {
		if _, err := os.Stat(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CALI_CONFIG_DIR"), "cali.yaml")); err == nil {
		t.Fatal("the config dir manifest was written")
	}
	code, out, errs := run("list", "-g", "--manifest", mf)
	if code != 0 || !strings.Contains(out, "x ") || !strings.Contains(out, repoURL) {
		t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("remove", "-g", "--manifest", mf, "x"); code != 0 {
		t.Fatalf("remove: code=%d out=%q err=%q", code, out, errs)
	}
	if b, err := os.ReadFile(filepath.Join(dot, "cali.lock")); err != nil || strings.Contains(string(b), `"x"`) {
		t.Fatalf("lock after remove: %q %v", b, err)
	}
}

func TestManifestFlagProject(t *testing.T) {
	repoURL, project := setup(t)
	if code, out, errs := run("add", "--manifest", filepath.Join("other", "cali.yaml"), repoURL, "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	other := filepath.Join(project, "other")
	for _, p := range []string{"cali.yaml", "cali.lock", filepath.Join(".claude", "skills", "x", "SKILL.md")} {
		if _, err := os.Stat(filepath.Join(other, p)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"cali.yaml", ".claude"} {
		if _, err := os.Stat(filepath.Join(project, p)); err == nil {
			t.Fatalf("%s written in the cwd project", p)
		}
	}
}

func TestManifestFlagProjectSymlinkRefused(t *testing.T) {
	repoURL, project := setup(t)
	other := filepath.Join(project, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(filepath.Dir(project), "real.yaml")
	if err := os.WriteFile(real, []byte("version: 1\ntargets: [claude]\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(other, "cali.yaml")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	code, _, errs := run("add", "--manifest", filepath.Join("other", "cali.yaml"), repoURL)
	if code != 1 || !strings.Contains(errs, "is a symlink") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

// The global manifest that status compares against ignores --manifest.
func TestManifestFlagProjectStatusIgnoresOverrideForGlobal(t *testing.T) {
	_, project := setup(t)
	home := os.Getenv("HOME")
	files := map[string]string{
		filepath.Join(project, "other", "cali.yaml"):                          "version: 1\ntargets: [claude]\nskills:\n  x:\n    source: github:o/r//skills/x\n    requires: [z]\n  z:\n    source: github:o/r//skills/z\n",
		filepath.Join(project, "other", ".claude", "skills", "x", "SKILL.md"): "---\nname: x\ndescription: X.\n---\n",
		// z exists only in the global dir and the real global manifest does not list it.
		filepath.Join(home, ".claude", "skills", "z", "SKILL.md"): "---\nname: z\ndescription: Z.\n---\n",
	}
	for p, c := range files {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	code, out, errs := run("status", "--offline", "--manifest", filepath.Join("other", "cali.yaml"))
	if code != 0 || strings.Contains(out, "shadowed") || !strings.Contains(out, "WARN x requires z (missing)") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
}

func TestManifestFlagInHomeRefusedInProject(t *testing.T) {
	repoURL, _ := setup(t)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	mf := filepath.Join(home, "cali.yaml")
	want := mf + ": a project manifest cannot sit in your home folder (its skills dirs are the global ones); use -g"
	for _, args := range [][]string{{"add", "--manifest", mf, repoURL}, {"init", "--manifest", mf}} {
		code, _, errs := run(args...)
		if code != 1 || errs != "cali: "+want+"\n" {
			t.Fatalf("%v: code=%d err=%q", args, code, errs)
		}
	}
	ents, err := os.ReadDir(home)
	if err != nil || len(ents) != 0 {
		t.Fatalf("home not empty: %v %v", ents, err)
	}
}

// Without --manifest, running in the home folder must not make home the project root either.
func TestHomeCwdRefusedInProject(t *testing.T) {
	repoURL, _ := setup(t)
	home := os.Getenv("HOME")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(home)
	want := filepath.Join(home, "cali.yaml") + ": a project manifest cannot sit in your home folder (its skills dirs are the global ones); use -g"
	for _, args := range [][]string{{"add", repoURL}, {"init"}} {
		code, _, errs := run(args...)
		if code != 1 || errs != "cali: "+want+"\n" {
			t.Fatalf("%v: code=%d err=%q", args, code, errs)
		}
	}
	ents, err := os.ReadDir(home)
	if err != nil || len(ents) != 0 {
		t.Fatalf("home not empty: %v %v", ents, err)
	}
}

func TestSyncSkipExitsNonZero(t *testing.T) {
	repoURL, project := setup(t)
	repo := filepath.FromSlash(strings.TrimPrefix(repoURL, "file://"))
	if err := os.MkdirAll(filepath.Join(repo, "skills", "y"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "skills", "y", "SKILL.md"), []byte("---\nname: [y]\ndescription: Y.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "bad y")
	cy := "version: 1\ntargets: [claude]\nskills:\n  x:\n    source: " + repoURL + "//skills/x\n  y:\n    source: " + repoURL + "//skills/y\n"
	if err := os.WriteFile(filepath.Join(project, "cali.yaml"), []byte(cy), 0o644); err != nil {
		t.Fatal(err)
	}
	// --frozen still fails before anything is skipped: the lock would change.
	if code, out, errs := run("sync", "--frozen"); code != 1 || out != "" || !strings.Contains(errs, "needs changes") {
		t.Fatalf("frozen: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := run("sync")
	if code != 1 || errs != "" || !strings.Contains(out, "installed x (claude)\n") || !strings.Contains(out, "y: skipped: ") {
		t.Fatalf("sync: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestUpdateSkipExitsNonZero(t *testing.T) {
	repoURL, _ := setup(t)
	repo := filepath.FromSlash(strings.TrimPrefix(repoURL, "file://"))
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	gitRun(t, repo, "rm", "-q", "-r", "skills/x")
	if err := os.WriteFile(filepath.Join(repo, "README"), []byte("x is gone\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "drop x")
	if code, out, errs := run("update"); code != 1 || errs != "" || !strings.HasPrefix(out, "x: skipped: ") {
		t.Fatalf("dry run: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("update", "--json"); code != 1 || errs != "" || !json.Valid([]byte(out)) || !strings.Contains(out, `"problem"`) {
		t.Fatalf("json: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("update", "--apply"); code != 1 || errs != "" || !strings.HasPrefix(out, "kept x: skipped: ") {
		t.Fatalf("apply: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestUpdateNotLockedExitsNonZero(t *testing.T) {
	repoURL, project := setup(t)
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	cy := filepath.Join(project, "cali.yaml")
	b, err := os.ReadFile(cy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cy, append(b, "  y:\n    source: "+repoURL+"//skills/y\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"update"}, {"update", "--json"}, {"update", "--apply"}} {
		if code, out, errs := run(args...); code != 1 || !strings.Contains(errs, "y: not locked, run cali sync\n") {
			t.Fatalf("%v: code=%d out=%q err=%q", args, code, out, errs)
		}
	}
}

// A skills repo that lists its own skills (.//path) in its committed
// manifest, installed globally as agent-skills' install.py does.
func TestSameRepoManifestGlobal(t *testing.T) {
	repoURL, _ := setup(t)
	repo := filepath.FromSlash(strings.TrimPrefix(repoURL, "file://"))
	mf := filepath.Join(repo, "cali.yaml")
	cali := func(wantCode int, want string, args ...string) {
		t.Helper()
		code, out, errs := run(append(args, "-g", "--manifest", mf)...)
		if code != wantCode || !strings.Contains(out+errs, want) {
			t.Fatalf("%v: code=%d out=%q err=%q", args, code, out, errs)
		}
	}
	cali(0, "installed x", "add", ".//skills/x", "--target", "claude")
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "manifest and lock")
	cali(0, "ok", "status", "--exit-code")

	// The skill changes in a commit that leaves the lock behind.
	if err := os.WriteFile(filepath.Join(repo, "skills", "x", "more.md"), []byte("more\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "more")
	cali(1, "outdated", "status", "--exit-code")
	cali(1, "behind the repo, run cali update: x", "sync", "--frozen")
	cali(0, "updated x - -> ", "update", "--apply")
	cali(0, "", "sync", "--frozen")
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude", "skills", "x", "more.md")); err != nil {
		t.Fatal(err)
	}
}

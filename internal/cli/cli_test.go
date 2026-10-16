package cli

import (
	"bytes"
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

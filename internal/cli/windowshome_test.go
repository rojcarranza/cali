package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/dirs"
)

// SCR-14 piece 4: in WSL, `cali sync -g --windows-home --target copilot` installs
// into the Windows home, so VS Code on Windows sees the skills.
func TestWindowsHomeFlag(t *testing.T) {
	repoURL, project := setup(t)
	winHome := filepath.Join(t.TempDir(), "Users", "me")
	if err := os.MkdirAll(winHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "copilot"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	if err := os.WriteFile(filepath.Join(project, "g.yaml"), mustRead(t, filepath.Join(project, "cali.yaml")), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	old := dirsFS
	dirsFS = dirs.FS{Exists: pathExists, Glob: func(p string) ([]string, error) {
		if p == "/mnt/*/Users/*" {
			return []string{"/mnt/c/Users/Public", winHome}, nil
		}
		return nil, nil
	}}
	t.Cleanup(func() { dirsFS = old })

	if code, _, errs := run("sync", "--windows-home"); code != 1 || !strings.Contains(errs, "needs -g") {
		t.Fatalf("without -g: code=%d err=%q", code, errs)
	}
	// One non-system profile is found by the glob; Public is skipped.
	if code, out, errs := run("sync", "-g", "--windows-home", "--manifest", filepath.Join(project, "g.yaml"), "--target", "copilot"); code != 0 {
		t.Fatalf("sync: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(winHome, ".copilot", "skills", "x", "SKILL.md")); err != nil {
		t.Fatalf("not installed in the Windows home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".copilot", "skills", "x")); !os.IsNotExist(err) {
		t.Fatal("also installed in the WSL home")
	}

	// SCR-14 piece 2 with piece 4: --prune removes the skill from the Windows home.
	if err := os.WriteFile(filepath.Join(project, "g.yaml"), []byte("version: 1\nskills: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run("sync", "-g", "--windows-home", "--prune", "--manifest", filepath.Join(project, "g.yaml")); code != 0 || !strings.Contains(out, "pruned x") {
		t.Fatalf("prune: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Lstat(filepath.Join(winHome, ".copilot", "skills", "x")); !os.IsNotExist(err) {
		t.Fatalf("skill still in the Windows home: %v", err)
	}

	// Two profiles: cali refuses and names CALI_WINDOWS_HOME.
	dirsFS.Glob = func(string) ([]string, error) { return []string{"/mnt/c/Users/a", "/mnt/c/Users/b"}, nil }
	if code, _, errs := run("list", "-g", "--windows-home"); code != 1 || !strings.Contains(errs, "CALI_WINDOWS_HOME") {
		t.Fatalf("several homes: code=%d err=%q", code, errs)
	}
	t.Setenv("CALI_WINDOWS_HOME", winHome)
	if code, _, errs := run("list", "-g", "--windows-home"); code != 0 {
		t.Fatalf("override: code=%d err=%q", code, errs)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

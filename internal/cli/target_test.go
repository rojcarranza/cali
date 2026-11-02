package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SCR-14 piece 1: --target limits sync, status, list, update and remove to one
// target, so `cali sync -g --target copilot` installs for Copilot only.
func TestTargetFlag(t *testing.T) {
	repoURL, project := setup(t)
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude", "--target", "copilot"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	claude := filepath.Join(project, ".claude", "skills", "x")
	copilot := filepath.Join(project, ".github", "skills", "x")
	for _, p := range []string{claude, copilot} {
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
	}

	if code, _, errs := run("sync", "--target", "vscode"); code != 1 || !strings.Contains(errs, "unknown target") {
		t.Fatalf("bad target: code=%d err=%q", code, errs)
	}

	if code, out, errs := run("sync", "--target", "copilot"); code != 0 || out != "installed x (copilot)\n" {
		t.Fatalf("sync: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(claude); !os.IsNotExist(err) {
		t.Fatal("sync --target copilot installed for claude")
	}

	code, out, _ := run("status", "--target", "copilot", "--offline")
	if code != 0 || strings.Contains(out, "claude") || !strings.Contains(out, "copilot") {
		t.Fatalf("status: code=%d out=%q", code, out)
	}
	code, out, _ = run("list", "--target", "copilot", "--offline", "--json")
	if code != 0 || !strings.Contains(out, `"copilot"`) || strings.Contains(out, `"claude"`) {
		t.Fatalf("list: code=%d out=%q", code, out)
	}
	if code, out, errs := run("update", "--target", "copilot"); code != 0 {
		t.Fatalf("update: code=%d out=%q err=%q", code, out, errs)
	}

	// remove --target copilot leaves the skill in the manifest for claude.
	if code, out, errs := run("remove", "x", "--target", "copilot"); code != 0 {
		t.Fatalf("remove: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(copilot); !os.IsNotExist(err) {
		t.Fatal("copilot folder not removed")
	}
	mf, err := os.ReadFile(filepath.Join(project, "cali.yaml"))
	if err != nil || !strings.Contains(string(mf), "x:") || !strings.Contains(string(mf), "claude") {
		t.Fatalf("manifest after partial remove: %v\n%s", err, mf)
	}
	if code, out, errs := run("sync"); code != 0 || out != "installed x (claude)\n" {
		t.Fatalf("sync after partial remove: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(copilot); !os.IsNotExist(err) {
		t.Fatal("copilot came back")
	}
}

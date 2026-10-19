package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// addSecondSkill commits skill y next to x in the fixture repo of setup.
func addSecondSkill(t *testing.T, repoURL string) {
	t.Helper()
	r := filepath.FromSlash(strings.TrimPrefix(repoURL, "file://"))
	if err := os.MkdirAll(filepath.Join(r, "skills", "y"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r, "skills", "y", "SKILL.md"), []byte("---\nname: y\ndescription: Y.\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, r, "add", ".")
	gitRun(t, r, "commit", "-q", "-m", "two")
}

func TestAddWholeRepoInstallsEverySkill(t *testing.T) {
	repoURL, project := setup(t)
	addSecondSkill(t, repoURL)
	if code, out, errs := run("add", repoURL, "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	for _, n := range []string{"x", "y"} {
		if _, err := os.Stat(filepath.Join(project, ".claude", "skills", n, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpdateBySourceSpec(t *testing.T) {
	repoURL, _ := setup(t)
	addSecondSkill(t, repoURL)
	if code, out, errs := run("add", repoURL, "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := run("update", repoURL, "--json")
	if code != 0 || !strings.Contains(out, `"x"`) || !strings.Contains(out, `"y"`) {
		t.Fatalf("update spec: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs = run("update", repoURL, "--skill", "x", "--json")
	if code != 0 || !strings.Contains(out, `"x"`) || strings.Contains(out, `"y"`) {
		t.Fatalf("update --skill: code=%d out=%q err=%q", code, out, errs)
	}
	if code, _, errs := run("update", repoURL, "--skill", "nope"); code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("update --skill nope: code=%d err=%q", code, errs)
	}
	if code, _, errs := run("update", "github:o/other"); code != 1 || !strings.Contains(errs, "no installed skill") {
		t.Fatalf("update unknown spec: code=%d err=%q", code, errs)
	}
}

func TestListAllScopes(t *testing.T) {
	repoURL, _ := setup(t)
	addSecondSkill(t, repoURL)
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("add", "-g", repoURL+"//skills/y", "--target", "claude"); code != 0 {
		t.Fatalf("add -g: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := run("list", "-a")
	if code != 0 || !strings.HasPrefix(out, "SCOPE") || !strings.Contains(out, "project  x") || !strings.Contains(out, "global   y") {
		t.Fatalf("list -a: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, _ := run("list"); code != 0 || strings.Contains(out, "y ") || strings.HasPrefix(out, "SCOPE") {
		t.Fatalf("list: code=%d out=%q", code, out)
	}
	if code, _, errs := run("list", "-a", "--manifest", "cali.yaml"); code != 1 || !strings.Contains(errs, "--manifest") {
		t.Fatalf("list -a --manifest: code=%d err=%q", code, errs)
	}
}

func TestFindSearchesBothScopes(t *testing.T) {
	repoURL, _ := setup(t)
	if code, out, errs := run("add", "-g", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add -g: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := run("find", "x", "--json")
	var got struct{ Hits []struct{ Name string } }
	if code != 0 || json.Unmarshal([]byte(out), &got) != nil || len(got.Hits) != 1 || got.Hits[0].Name != "x" {
		t.Fatalf("find: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestShowFallsBackToGlobal(t *testing.T) {
	repoURL, _ := setup(t)
	if code, out, errs := run("add", "-g", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add -g: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("show", "x"); code != 0 || !strings.Contains(out, repoURL) {
		t.Fatalf("show: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestRemovePrintsRestore(t *testing.T) {
	repoURL, _ := setup(t)
	if code, out, errs := run("add", "-g", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add -g: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := run("remove", "-g", "x")
	if code != 0 || !strings.Contains(out, "to restore: cali add -g "+repoURL+"//skills/x --target claude") {
		t.Fatalf("remove: code=%d out=%q err=%q", code, out, errs)
	}
}

func TestSyncGlobalUsesProjectManifest(t *testing.T) {
	repoURL, project := setup(t)
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("sync", "-g"); code != 0 || !strings.Contains(out, "installed x (claude)") {
		t.Fatalf("sync -g: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude", "skills", "x", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CALI_CONFIG_DIR"), "cali.yaml")); err == nil {
		t.Fatal("the config dir manifest was written")
	}
	if _, err := os.Stat(filepath.Join(project, "cali.lock")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncWithoutManifest(t *testing.T) {
	repoURL, project := setup(t)
	t.Setenv("CALI_DEFAULT_SOURCE", repoURL)

	code, out, errs := run("sync")
	if code != 0 || !strings.Contains(out, "cali add "+repoURL) {
		t.Fatalf("sync, no terminal: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(project, "cali.yaml")); err == nil {
		t.Fatal("cali.yaml written without a yes")
	}

	oldI, oldC := interactive, confirm
	t.Cleanup(func() { interactive, confirm = oldI, oldC })
	interactive = func(io.Writer) bool { return true }
	var asked string
	confirm = func(q string) (bool, error) { asked = q; return true, nil }
	if code, out, errs := run("sync"); code != 0 || !strings.Contains(asked, repoURL) || !strings.Contains(out, "x") {
		t.Fatalf("sync, yes: code=%d out=%q err=%q asked=%q", code, out, errs, asked)
	}
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "x", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

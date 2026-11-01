package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/skillfs"
)

// Intent (SCR-14 piece 8, AXI rule 6): in agent mode cali never prompts and a
// usage error exits 2; without agent mode the first-run prompt and exit 1 stay.
func TestAgentModeNeverPromptsAndExits2OnUsage(t *testing.T) {
	repoURL, project := setup(t)
	t.Setenv("CALI_DEFAULT_SOURCE", repoURL)
	oldI, oldC := interactive, confirm
	t.Cleanup(func() { interactive, confirm = oldI, oldC })
	interactive = func(io.Writer) bool { return true }
	confirm = func(string) (bool, error) { t.Fatal("prompted in agent mode"); return false, nil }

	code, out, errs := run("sync", "--agent")
	if code != 0 || !strings.Contains(out, "cali add "+repoURL) {
		t.Fatalf("sync --agent: code=%d out=%q err=%q", code, out, errs)
	}
	if _, err := os.Stat(filepath.Join(project, "cali.yaml")); err == nil {
		t.Fatal("cali.yaml written without a yes")
	}
	t.Setenv("CALI_AGENT", "1")
	if code, _, _ := run("sync"); code != 0 {
		t.Fatalf("sync with CALI_AGENT: code=%d", code)
	}
	for _, args := range [][]string{{"list", "--nope"}, {"nope"}, {"list", "extra"}} {
		if code, _, errs := run(args...); code != 2 || errs == "" {
			t.Fatalf("%v: code=%d err=%q, want 2", args, code, errs)
		}
	}
	if code, _, _ := run("show", "missing"); code != 1 {
		t.Fatalf("runtime failure must exit 1, got %d", code)
	}
	t.Setenv("CALI_AGENT", "")
	if code, _, _ := run("list", "--nope"); code != 1 {
		t.Fatalf("outside agent mode a usage error exits 1, got %d", code)
	}
}

// Intent (AXI rules 4, 5, 9): agent-mode list and status open with totals,
// say "0 skills" when empty, and end with the next command.
func TestAgentModeTotalsAndHints(t *testing.T) {
	repoURL, _ := setup(t)
	t.Setenv("CALI_AGENT", "1")
	if code, out, errs := run("list"); code != 0 || out != "0 skills\nnext: cali add <source>\n" {
		t.Fatalf("empty list: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("add", repoURL+"//skills/x", "--target", "claude"); code != 0 {
		t.Fatalf("add: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("list", "--offline"); code != 0 || !strings.HasPrefix(out, "1 skills\n") {
		t.Fatalf("list: code=%d out=%q err=%q", code, out, errs)
	}
	code, out, errs := run("status", "--offline")
	if code != 0 || !strings.HasPrefix(out, "1 skills: 1 ok\n") || strings.Contains(out, "next:") {
		t.Fatalf("status: code=%d out=%q err=%q", code, out, errs)
	}
	if err := os.RemoveAll(filepath.Join(".claude", "skills", "x")); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := run("status", "--offline"); !strings.HasPrefix(out, "1 skills: 1 missing\n") || !strings.HasSuffix(out, "next: cali sync\n") {
		t.Fatalf("status missing: out=%q", out)
	}
	t.Setenv("CALI_AGENT", "")
	if _, out, _ := run("list", "--offline"); strings.Contains(out, "1 skills") {
		t.Fatalf("totals leaked outside agent mode: %q", out)
	}
}

// Intent (AXI rule 7): the cali skill shipped in this repo is a valid skill.
func TestShippedSkillHasFrontmatter(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "skills", "cali", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	fm, err := skillfs.ParseFrontmatter(b)
	if err != nil || fm.Name != "cali" || fm.Description == "" {
		t.Fatalf("frontmatter: %+v err=%v", fm, err)
	}
}

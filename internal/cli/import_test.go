package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/manifest"
)

const importAPM = `targets: [claude]
dependencies:
  apm:
    - o/r/skills/same#aaa
    - o/r/skills/other#bbb
    - o/r/skills/fresh#ccc
  mcp:
    - some/server
`

func TestImportMergesAndLeavesUnmanagedFoldersAlone(t *testing.T) {
	_, project := setup(t)
	apmFile := filepath.Join(project, "apm.yml")
	if err := os.WriteFile(apmFile, []byte(importAPM), 0o644); err != nil {
		t.Fatal(err)
	}
	existing := "# mine\nversion: 1\ntargets: [claude, copilot]\nskills:\n" +
		"  same: {source: 'github:o/r//skills/same', ref: aaa}\n" +
		"  other: {source: 'github:x/y//skills/other'}\n"
	if err := os.WriteFile(filepath.Join(project, "cali.yaml"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(project, ".claude", "skills", "apm-left-over")
	if err := os.MkdirAll(extra, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extra, "SKILL.md"), []byte("---\nname: apm-left-over\ndescription: D.\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, errs := run("import", apmFile)
	if code != 0 || out != "imported 1 skill(s); run cali sync to install\n" {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	for _, w := range []string{"warning: mcp server some/server is not managed by cali",
		"warning: skipped other: already in cali.yaml from github:x/y//skills/other"} {
		if !strings.Contains(errs, w) {
			t.Errorf("stderr lacks %q: %q", w, errs)
		}
	}
	m, err := manifest.Read(filepath.Join(project, "cali.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Skills) != 3 || m.Skills["fresh"].Source != "github:o/r//skills/fresh" || m.Skills["fresh"].Ref != "ccc" ||
		m.Skills["other"].Source != "github:x/y//skills/other" || len(m.Targets) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(project, "cali.yaml")); !strings.HasPrefix(string(b), "# mine") {
		t.Errorf("comment lost: %q", b)
	}
	if _, err := os.Stat(filepath.Join(extra, "SKILL.md")); err != nil {
		t.Fatalf("unmanaged folder touched: %v", err)
	}
	if _, st, _ := run("status", "--offline"); !strings.Contains(st, "apm-left-over") || !strings.Contains(st, "unmanaged") {
		t.Errorf("status = %q", st)
	}
}

func TestImportCreatesManifest(t *testing.T) {
	_, project := setup(t)
	apmFile := filepath.Join(project, "apm.yml")
	if err := os.WriteFile(apmFile, []byte(importAPM), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run("import", apmFile); code != 0 || !strings.HasPrefix(out, "imported 3 ") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errs)
	}
	m, err := manifest.Read(filepath.Join(project, "cali.yaml"))
	if err != nil || len(m.Skills) != 3 || len(m.Targets) != 1 || m.Targets[0] != "claude" {
		t.Fatalf("manifest = %+v, err = %v", m, err)
	}
}

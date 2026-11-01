package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func claudeFixture(t *testing.T) string {
	t.Helper()
	c := t.TempDir()
	w := func(p, body string) {
		p = filepath.Join(c, p)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("plugins/installed_plugins.json", `{"version":2,"plugins":{"sp@official":[{"scope":"user","installPath":"`+filepath.Join(c, "cache/sp")+`","gitCommitSha":"abc1234def"}]}}`)
	w("plugins/known_marketplaces.json", `{"official":{"source":{"source":"github","repo":"o/r"},"installLocation":"`+filepath.Join(c, "mk")+`"}}`)
	w("mk/.claude-plugin/marketplace.json", `{"plugins":[{"name":"sp","source":"./plugins/sp"}]}`)
	w("cache/sp/skills/one/SKILL.md", "x")
	w("cache/sp/skills/two/SKILL.md", "x")
	return c
}

// SCR-14 piece 6: import claude writes the chosen skills, pinned, to the manifest.
func TestImportClaude(t *testing.T) {
	_, project := setup(t)
	c := claudeFixture(t)

	code, out, _ := run("import", "claude", "--claude-dir", c)
	if code != 0 || !strings.Contains(out, "2 skills found") || !strings.Contains(out, "one\tsp@official") {
		t.Fatalf("list: code=%d out=%q", code, out)
	}
	if _, err := os.Stat(filepath.Join(project, "cali.yaml")); err == nil {
		t.Fatal("listing wrote a manifest")
	}

	code, out, errs := run("import", "claude", "--claude-dir", c, "--skill", "one")
	if code != 0 || !strings.Contains(out, "1 added, 0 kept") {
		t.Fatalf("import: code=%d out=%q err=%q", code, out, errs)
	}
	b, _ := os.ReadFile(filepath.Join(project, "cali.yaml"))
	if !strings.Contains(string(b), "github:o/r//plugins/sp/skills/one") || !strings.Contains(string(b), "abc1234def") || strings.Contains(string(b), "two:") {
		t.Fatalf("manifest:\n%s", b)
	}

	code, out, _ = run("import", "claude", "--claude-dir", c, "--all")
	if code != 0 || !strings.Contains(out, "kept one") || !strings.Contains(out, "1 added, 1 kept") {
		t.Fatalf("all: code=%d out=%q", code, out)
	}
	if code, _, errs := run("import", "claude", "--claude-dir", c, "--skill", "nope"); code != 1 || !strings.Contains(errs, "no skill") {
		t.Fatalf("unknown: code=%d err=%q", code, errs)
	}
}

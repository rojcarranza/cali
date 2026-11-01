package claudeimport

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Intent (SCR-14 piece 6): a fixture shaped like a real installed_plugins.json
// gives the expected pinned cali.yaml entries.
func TestScan(t *testing.T) {
	c := t.TempDir()
	mk := filepath.Join(c, "plugins", "marketplaces", "official")
	write(t, filepath.Join(c, "plugins", "installed_plugins.json"), `{"version":2,"plugins":{
 "superpowers@official":[{"scope":"user","installPath":"`+filepath.Join(c, "cache", "sp")+`","gitCommitSha":"aaa111"}],
 "ext@official":[{"scope":"user","installPath":"`+filepath.Join(c, "cache", "ext")+`","gitCommitSha":"bbb222"}],
 "off@official":[{"scope":"user","installPath":"`+filepath.Join(c, "cache", "off")+`","gitCommitSha":"ccc333"}],
 "proj@official":[{"scope":"project","projectPath":"/x","installPath":"/x","gitCommitSha":"ddd444"}],
 "zdup@official":[{"scope":"user","installPath":"`+filepath.Join(c, "cache", "zdup")+`","gitCommitSha":"eee555"}]}}`)
	write(t, filepath.Join(c, "plugins", "known_marketplaces.json"),
		`{"official":{"source":{"source":"github","repo":"anthropics/plugins"},"installLocation":"`+mk+`"}}`)
	write(t, filepath.Join(mk, ".claude-plugin", "marketplace.json"), `{"plugins":[
 {"name":"superpowers","source":"./plugins/superpowers"},
 {"name":"ext","source":{"source":"git-subdir","url":"https://github.com/acme/ext.git","path":"sub","sha":"x"}},
 {"name":"off","source":"./off"},{"name":"zdup","source":"./zdup"}]}`)
	write(t, filepath.Join(c, "settings.json"), `{"enabledPlugins":{"off@official":false,"superpowers@official":true}}`)
	write(t, filepath.Join(c, "cache", "sp", "skills", "brainstorming", "SKILL.md"), "x")
	write(t, filepath.Join(c, "cache", "sp", "skills", "notaskill", "README.md"), "x")
	write(t, filepath.Join(c, "cache", "ext", ".claude-plugin", "plugin.json"), `{"skills":"./lib/skills"}`)
	write(t, filepath.Join(c, "cache", "ext", "lib", "skills", "deploy", "SKILL.md"), "x")
	write(t, filepath.Join(c, "cache", "zdup", "skills", "brainstorming", "SKILL.md"), "x")

	res, err := Scan(c)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Candidate{
		"brainstorming": {Name: "brainstorming", Plugin: "superpowers@official", Source: "github:anthropics/plugins//plugins/superpowers/skills/brainstorming", Ref: "aaa111"},
		"deploy":        {Name: "deploy", Plugin: "ext@official", Source: "github:acme/ext//sub/lib/skills/deploy", Ref: "bbb222"},
	}
	if len(res.Candidates) != len(want) {
		t.Fatalf("candidates = %+v, notes %v", res.Candidates, res.Notes)
	}
	for _, got := range res.Candidates {
		if got != want[got.Name] {
			t.Errorf("%s = %+v, want %+v", got.Name, got, want[got.Name])
		}
		if err := Valid(got); err != nil {
			t.Errorf("%s: %v", got.Name, err)
		}
	}
	if len(res.Notes) != 3 { // off, proj, dup
		t.Errorf("notes = %v", res.Notes)
	}
}

func TestScanNoClaude(t *testing.T) {
	if _, err := Scan(t.TempDir()); err == nil {
		t.Fatal("want an error when Claude has no plugins file")
	}
}

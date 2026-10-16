package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, project string) {
	t.Helper()
	files := map[string]string{
		"cali.yaml": "version: 1\ntargets: [claude, copilot]\nskills:\n  b:\n    source: github:o/r//skills/b\n    requires: [a]\n  a:\n    source: github:o/r//skills/a\n    targets: [claude]\n",
		"cali.lock": `{"version":1,"skills":{"a":{"source":"github.com/o/r","path":"skills/a","ref":"main","commit":"0123456789abcdef","tree":"t1"}}}`,
		filepath.Join(".claude", "skills", "b", "SKILL.md"): "---\nname: b\ndescription: B.\nmetadata:\n  cali-requires: \"c, a\"\n---\nbody\n",
	}
	for p, c := range files {
		full := filepath.Join(project, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListOffline(t *testing.T) {
	_, project := setup(t)
	writeFixture(t, project)
	t.Setenv("PATH", t.TempDir())

	code, out, errs := run("list")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "NAME") ||
		!strings.Contains(lines[1], "0123456") || !strings.Contains(lines[1], "claude") ||
		!strings.HasPrefix(lines[2], "b") || !strings.Contains(lines[2], "claude,copilot") || !strings.Contains(lines[2], " -") {
		t.Fatalf("out = %q", out)
	}
	if strings.Contains(lines[1], "0123456789") {
		t.Fatalf("commit not shortened: %q", lines[1])
	}

	code, out, errs = run("list", "--json")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	var got struct {
		SchemaVersion int `json:"schema_version"`
		Skills        []struct {
			Name, Source, Commit string
			Targets              []string
		} `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if got.SchemaVersion != 1 || len(got.Skills) != 2 || got.Skills[0].Name != "a" || got.Skills[1].Commit != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestShow(t *testing.T) {
	_, project := setup(t)
	writeFixture(t, project)

	code, out, errs := run("show", "b", "--json")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errs)
	}
	var got struct {
		Name    string
		Targets []struct {
			Target, Dir string
			Installed   bool
		}
		Requires []string
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if got.Name != "b" || len(got.Targets) != 2 || !got.Targets[0].Installed || got.Targets[1].Installed ||
		strings.Join(got.Requires, ",") != "a,c" {
		t.Fatalf("got %+v", got)
	}

	code, out, _ = run("show", "a")
	if code != 0 || !strings.Contains(out, "0123456789abcdef") || !strings.Contains(out, "t1") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if code, _, errs := run("show", "nope"); code != 1 || !strings.Contains(errs, "nope") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

func TestListNoManifest(t *testing.T) {
	setup(t)
	for _, args := range [][]string{{"list"}, {"list", "-g"}} {
		code, out, errs := run(args...)
		if code != 0 || strings.Count(strings.TrimSpace(out), "\n") != 0 {
			t.Fatalf("%v: code=%d out=%q err=%q", args, code, out, errs)
		}
		code, out, errs = run(append(args, "--json")...)
		var got struct {
			Skills []struct{} `json:"skills"`
		}
		if code != 0 || json.Unmarshal([]byte(out), &got) != nil || got.Skills == nil || len(got.Skills) != 0 {
			t.Fatalf("%v --json: code=%d out=%q err=%q", args, code, out, errs)
		}
	}
	if code, _, errs := run("show", "x"); code != 1 || !strings.Contains(errs, "no skill named x") {
		t.Fatalf("code=%d err=%q", code, errs)
	}
}

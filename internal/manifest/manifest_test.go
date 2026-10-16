package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cali.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const sample = `version: 1
targets: [claude, copilot]
skills:
  code-review:
    source: github:rojcarranza/agent-skills//skills/code-review
    ref: main
  brainstorming:
    source: github:obra/superpowers//skills/brainstorming
    ref: v5.0.0
    targets: [claude]
    requires: [writing-plans]
`

func TestReadUnknownKeyIsError(t *testing.T) {
	p := writeFile(t, "version: 1\ntaregts: [claude]\nskills: {}\n")
	_, err := Read(p)
	if err == nil {
		t.Fatal("want error for unknown key")
	}
	if !strings.Contains(err.Error(), "taregts") {
		t.Errorf("error should name the key, got %q", err)
	}
}

func TestReadValid(t *testing.T) {
	m, err := Read(writeFile(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != 1 || !reflect.DeepEqual(m.Targets, []string{"claude", "copilot"}) {
		t.Errorf("unexpected header: %+v", m)
	}
	b := m.Skills["brainstorming"]
	if b.Ref != "v5.0.0" || !reflect.DeepEqual(b.Requires, []string{"writing-plans"}) {
		t.Errorf("unexpected skill: %+v", b)
	}
}

func TestReadRejectsBadVersion(t *testing.T) {
	if _, err := Read(writeFile(t, "version: 2\nskills: {}\n")); err == nil {
		t.Fatal("want error for version 2")
	}
}

func TestReadRejectsBadSource(t *testing.T) {
	p := writeFile(t, "version: 1\nskills:\n  x:\n    source: ftp://user:hunter2@host/x\n")
	_, err := Read(p)
	if err == nil {
		t.Fatal("want error for bad source")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Errorf("error leaks the raw source: %q", err)
	}
}

func TestReadRejectsMissingSourceAndBadName(t *testing.T) {
	if _, err := Read(writeFile(t, "version: 1\nskills:\n  x:\n    ref: main\n")); err == nil {
		t.Error("want error for missing source")
	}
	bad := "version: 1\nskills:\n  Bad_Name:\n    source: github:a/b\n"
	if _, err := Read(writeFile(t, bad)); err == nil {
		t.Error("want error for invalid skill name")
	}
}

func TestReadValidatesTargets(t *testing.T) {
	top := "version: 1\ntargets: [claude, vim]\nskills: {}\n"
	if _, err := Read(writeFile(t, top)); err == nil {
		t.Error("want error for bad default target")
	}
	per := "version: 1\nskills:\n  x:\n    source: github:a/b\n    targets: [cursor]\n"
	if _, err := Read(writeFile(t, per)); err == nil {
		t.Error("want error for bad per-skill target")
	}
}

func TestTargetsForOverride(t *testing.T) {
	m, err := Read(writeFile(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.TargetsFor("brainstorming"); !reflect.DeepEqual(got, []string{"claude"}) {
		t.Errorf("override: got %v", got)
	}
	if got := m.TargetsFor("code-review"); !reflect.DeepEqual(got, []string{"claude", "copilot"}) {
		t.Errorf("default: got %v", got)
	}
}

func TestSkillNameValid(t *testing.T) {
	ok := []string{"a", "code-review", "x1", "0", strings.Repeat("a", 64)}
	bad := []string{"", "Code", "a_b", "a b", "a/b", "..", strings.Repeat("a", 65)}
	for _, n := range ok {
		if !SkillNameValid(n) {
			t.Errorf("%q should be valid", n)
		}
	}
	for _, n := range bad {
		if SkillNameValid(n) {
			t.Errorf("%q should be invalid", n)
		}
	}
}

func TestWriteRoundTrip(t *testing.T) {
	m, err := Read(writeFile(t, sample))
	if err != nil {
		t.Fatal(err)
	}
	m.Sources = []string{"github:obra/superpowers"}
	p := filepath.Join(t.TempDir(), "out.yaml")
	if err := Write(p, m); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, m)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
}

const commented = `# Project skills, hand edited.
version: 1
targets: [claude, copilot] # default
skills:
  # the review skill
  code-review:
    source: github:rojcarranza/agent-skills//skills/code-review
    ref: main
`

func TestWritePreservesComments(t *testing.T) {
	p := writeFile(t, commented)
	m, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	m.Skills["brainstorming"] = Skill{
		Source:  "github:obra/superpowers//skills/brainstorming",
		Ref:     "v5.0.0",
		Targets: []string{"claude"},
	}
	if err := Write(p, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	for _, want := range []string{"# Project skills, hand edited.", "# the review skill", "# default"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("comment %q lost:\n%s", want, raw)
		}
	}
	got, err := Read(p)
	if err != nil {
		t.Fatalf("read back: %v\n%s", err, raw)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("mismatch:\n got %+v\nwant %+v", got, m)
	}
}

func TestWriteRemovesAndChanges(t *testing.T) {
	p := writeFile(t, sample)
	m, _ := Read(p)
	delete(m.Skills, "code-review")
	b := m.Skills["brainstorming"]
	b.Ref = "1.0" // looks like a float; must stay a string
	b.Targets = nil
	b.Requires = nil
	m.Skills["brainstorming"] = b
	if err := Write(p, m); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, m) {
		raw, _ := os.ReadFile(p)
		t.Errorf("mismatch:\n got %+v\nwant %+v\n%s", got, m, raw)
	}
}

func TestWriteRejectsInvalid(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cali.yaml")
	m := Manifest{Version: 1, Skills: map[string]Skill{"x": {Source: "github:a/b", Targets: []string{"vim"}}}}
	if err := Write(p, m); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(p); err == nil {
		t.Error("file must not be created")
	}
}

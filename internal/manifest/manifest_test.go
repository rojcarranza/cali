package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/source"
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
	ok = append(ok, "con-x", "com0", "com10", "lpt", "nul1x", "auxiliary")
	bad := []string{"", "Code", "a_b", "a b", "a/b", "..", strings.Repeat("a", 65),
		"con", "prn", "aux", "nul", "com1", "com5", "com9", "lpt1", "lpt9"}
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

func TestWriteWithAllowsFileScheme(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cali.yaml")
	m := Manifest{Version: 1, Targets: []string{"claude"}, Skills: map[string]Skill{
		"x": {Source: "file:///tmp/repo//skills/x"},
	}}
	if err := Write(p, m); err == nil {
		t.Fatal("Write accepted a file:// source without the option")
	}
	if err := WriteWith(p, m, source.Options{AllowFile: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(p); err == nil {
		t.Fatal("Read accepted a file:// source without the option")
	}
	got, err := ReadWith(p, source.Options{AllowFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Skills["x"].Source != "file:///tmp/repo//skills/x" {
		t.Fatalf("got %+v", got)
	}
}

func TestReadRejectsRefOnSameRepoSource(t *testing.T) {
	p := writeFile(t, "version: 1\nskills:\n  x:\n    source: .//skills/x\n    ref: main\n")
	if _, err := Read(p); err == nil || !strings.Contains(err.Error(), "no ref") {
		t.Fatalf("err = %v, want one saying a same-repo source takes no ref", err)
	}
}

// handEdited mirrors a real hand-edited manifest: two spaces before inline
// comments, odd spacing in a flow list and a trailing block of commented-out
// entries. Writes must leave every line they do not change byte for byte.
const handEdited = `# Skills for this machine.
version: 1
targets: [claude, copilot]
skills:
  # Own skills.
  own:
    source: .//skills/own
  caveman:  # Claude: caveman plugin
    source: https://github.com/juliusbrussee/caveman//skills/caveman
    ref: 2e08b9177c07bb7249a8a2d1a6758e5db281d002
    targets: [copilot]
  debugging:  # Claude: superpowers:systematic-debugging
    source: github:addyosmani/agent-skills//skills/debugging
    targets:    [copilot]
  # Optional, uncomment to enable:
  # extra:
  #   source: github:x/y//skills/extra
`

func writeHandEdited(t *testing.T, edit func(m *Manifest)) string {
	t.Helper()
	p := writeFile(t, handEdited)
	m, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	edit(&m)
	if err := Write(p, m); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatalf("read back: %v\n%s", err, raw)
	}
	if !reflect.DeepEqual(got, m) {
		t.Errorf("mismatch:\n got %+v\nwant %+v", got, m)
	}
	return string(raw)
}

func TestWriteAddKeepsExistingLines(t *testing.T) {
	got := writeHandEdited(t, func(m *Manifest) {
		m.Skills["impeccable"] = Skill{Source: "github:pbakaus/impeccable//skills/impeccable", Ref: "abc123", Targets: []string{"claude"}}
	})
	want := strings.Replace(handEdited, "  # Optional,",
		"  impeccable:\n    source: github:pbakaus/impeccable//skills/impeccable\n    ref: abc123\n    targets: [claude]\n  # Optional,", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteChangeTouchesOnlyChangedLine(t *testing.T) {
	got := writeHandEdited(t, func(m *Manifest) {
		s := m.Skills["caveman"]
		s.Ref = "ffffffffffffffffffffffffffffffffffffffff"
		m.Skills["caveman"] = s
	})
	want := strings.Replace(handEdited, "2e08b9177c07bb7249a8a2d1a6758e5db281d002", "ffffffffffffffffffffffffffffffffffffffff", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteRemoveKeepsOtherLines(t *testing.T) {
	got := writeHandEdited(t, func(m *Manifest) { delete(m.Skills, "debugging") })
	want := strings.Replace(handEdited,
		"  debugging:  # Claude: superpowers:systematic-debugging\n    source: github:addyosmani/agent-skills//skills/debugging\n    targets:    [copilot]\n", "", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteRemoveDropsHeadComment(t *testing.T) {
	got := writeHandEdited(t, func(m *Manifest) { delete(m.Skills, "own") })
	want := strings.Replace(handEdited, "  # Own skills.\n  own:\n    source: .//skills/own\n", "", 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestWriteSpliceKeepsCRLF(t *testing.T) {
	crlf := strings.ReplaceAll(strings.TrimSuffix(handEdited, "\n"), "\n", "\r\n") // no final newline
	p := writeFile(t, crlf)
	m, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	m.Skills["impeccable"] = Skill{Source: "github:pbakaus/impeccable//skills/impeccable"}
	if err := Write(p, m); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	want := strings.Replace(crlf, "  # Optional,", "  impeccable:\r\n    source: github:pbakaus/impeccable//skills/impeccable\r\n  # Optional,", 1) + "\r\n"
	if string(raw) != want {
		t.Errorf("got %q\nwant %q", raw, want)
	}
}

func TestWriteHeaderChangeFallsBackToFullEncode(t *testing.T) {
	got := writeHandEdited(t, func(m *Manifest) { m.Targets = []string{"claude"} })
	if !strings.Contains(got, "targets: [claude]\n") || !strings.Contains(got, "# Claude: caveman plugin") {
		t.Errorf("got:\n%s", got)
	}
}

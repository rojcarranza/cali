package lock

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func entry(n string) Entry {
	return Entry{
		Source: "https://github.com/o/" + n,
		Path:   "skills/" + n,
		Ref:    "main",
		Commit: strings.Repeat("a", 40),
		Tree:   strings.Repeat("b", 40),
	}
}

func TestMarshalSortedAndStable(t *testing.T) {
	names := []string{"zeta", "alpha", "mid", "beta"}
	a := Lock{Version: 1, Skills: map[string]Entry{}}
	for _, n := range names {
		a.Skills[n] = entry(n)
	}
	b := Lock{Version: 1, Skills: map[string]Entry{}}
	for i := len(names) - 1; i >= 0; i-- {
		b.Skills[names[i]] = entry(names[i])
	}
	ba, bb := a.Marshal(), b.Marshal()
	if !bytes.Equal(ba, bb) {
		t.Fatalf("different bytes for same content:\n%s\n%s", ba, bb)
	}
	s := string(ba)
	if !strings.HasSuffix(s, "}\n") || strings.HasSuffix(s, "\n\n") {
		t.Errorf("want exactly one trailing newline, got %q", s[len(s)-3:])
	}
	if strings.Contains(s, "\r") {
		t.Error("output contains CR")
	}
	if !strings.Contains(s, "\n  \"skills\": {\n    \"alpha\": {\n      \"source\"") {
		t.Errorf("want 2-space indent, got:\n%s", s)
	}
	if strings.Index(s, "\"alpha\"") > strings.Index(s, "\"beta\"") ||
		strings.Index(s, "\"mid\"") > strings.Index(s, "\"zeta\"") {
		t.Errorf("keys not sorted:\n%s", s)
	}
}

func TestMarshalDoesNotEscapeHTML(t *testing.T) {
	l := Lock{Version: 1, Skills: map[string]Entry{"x": {Ref: "a&b<c>"}}}
	if !strings.Contains(string(l.Marshal()), "a&b<c>") {
		t.Errorf("html escaped: %s", l.Marshal())
	}
}

func TestMarshalNilSkills(t *testing.T) {
	got := string(Lock{Version: 1}.Marshal())
	want := "{\n  \"version\": 1,\n  \"skills\": {}\n}\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cali.lock")
	l := Lock{Version: 1, Skills: map[string]Entry{"a": entry("a"), "b": entry("b")}}
	if err := Write(p, l); err != nil {
		t.Fatal(err)
	}
	got, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, l) {
		t.Errorf("got %+v want %+v", got, l)
	}
	// overwrite existing file
	l.Skills["c"] = entry("c")
	if err := Write(p, l); err != nil {
		t.Fatal(err)
	}
	got, err = Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Skills) != 3 {
		t.Errorf("overwrite failed: %+v", got)
	}
	ents, err := os.ReadDir(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("temp file left behind: %v", ents)
	}
}

func TestWriteFailureLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "cali.lock")
	if err := os.Mkdir(p, 0o755); err != nil { // rename onto a non-empty directory fails
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "x"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(p, Lock{Version: 1}); err == nil {
		t.Fatal("want error")
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("temp file left behind: %v", ents)
	}
}

func TestReadMissingFile(t *testing.T) {
	l, err := Read(filepath.Join(t.TempDir(), "nope.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if l.Version != 1 || l.Skills == nil || len(l.Skills) != 0 {
		t.Errorf("got %+v", l)
	}
}

func TestReadRejectsUnknownVersion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cali.lock")
	if err := os.WriteFile(p, []byte(`{"version": 2, "skills": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(p); err == nil {
		t.Fatal("want error for version 2")
	}
}

func TestReadRejectsBadInput(t *testing.T) {
	for name, body := range map[string]string{
		"unknown top field":   `{"version":1,"skills":{},"extra":1}`,
		"unknown entry field": `{"version":1,"skills":{"a":{"secret-token":"x"}}}`,
		"malformed":           `{"version":`,
		"trailing data":       `{"version":1,"skills":{}} {}`,
		"empty":               ``,
	} {
		p := filepath.Join(t.TempDir(), "cali.lock")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Read(p)
		if err == nil {
			t.Errorf("%s: want error", name)
			continue
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Errorf("%s: error leaks file content: %v", name, err)
		}
	}
}

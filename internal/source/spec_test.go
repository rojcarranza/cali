package source

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Spec
	}{
		{"github:obra/superpowers//skills/x@v1.2.0", Spec{URL: "https://github.com/obra/superpowers", Path: "skills/x", Ref: "v1.2.0"}},
		{"github:o/r", Spec{URL: "https://github.com/o/r"}},
		{"github:o/r@main", Spec{URL: "https://github.com/o/r", Ref: "main"}},
		{"https://h/o/r.git//p@abc", Spec{URL: "https://h/o/r.git", Path: "p", Ref: "abc"}},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		in, wantMsg string
	}{
		{"github:o/r//../x", ".."},
		{"github:o/r//skills/../../x", ".."},
		{"github:o/r///abs", "leading"},
		{"https://user:pass@h/o/r", "user:pass"},
		{"https://user:pass@h/o/r//p", "user:pass"},
		{"ssh://git@h/o/r", "ssh"},
		{"file:///tmp/repo", "file"},
		{"github:/r", "owner"},
		{"github:o/", "repo"},
		{"github:o", "repo"},
		{"https://h//p", "owner"},
	}
	for _, c := range cases {
		_, err := Parse(c.in)
		if err == nil {
			t.Errorf("Parse(%q) expected error", c.in)
			continue
		}
		if !strings.Contains(err.Error(), c.wantMsg) {
			t.Errorf("Parse(%q) error %q does not name %q", c.in, err, c.wantMsg)
		}
	}
}

func TestParseWithAllowFile(t *testing.T) {
	got, err := ParseWith("file:///tmp/repo//skills/x@v1", Options{AllowFile: true})
	if err != nil {
		t.Fatalf("ParseWith error: %v", err)
	}
	want := Spec{URL: "file:///tmp/repo", Path: "skills/x", Ref: "v1"}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if _, err := ParseWith("ssh://git@h/o/r", Options{AllowFile: true}); err == nil {
		t.Error("ssh must still be rejected with AllowFile")
	}
}

func TestStringRoundTrip(t *testing.T) {
	for _, in := range []string{
		"github:obra/superpowers//skills/x@v1.2.0",
		"github:o/r",
		"github:o/r@main",
		"https://h/o/r.git//p@abc",
	} {
		s, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		again, err := Parse(s.String())
		if err != nil {
			t.Fatalf("Parse(String(%q)=%q): %v", in, s.String(), err)
		}
		if again != s {
			t.Errorf("round trip %q: %+v != %+v", in, again, s)
		}
	}
	s, _ := Parse("github:o/r//p@v1")
	if s.String() != "https://github.com/o/r//p@v1" {
		t.Errorf("String() = %q", s.String())
	}
}

func TestKey(t *testing.T) {
	cases := map[string]string{
		"github:obra/superpowers//x": "github.com/obra/superpowers",
		"https://h/o/r.git//p@abc":   "h/o/r",
	}
	for in, want := range cases {
		s, err := Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := s.Key(); got != want {
			t.Errorf("Key(%q) = %q, want %q", in, got, want)
		}
	}
}

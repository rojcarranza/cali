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
		{"https://h/o/r.git//p@abc", Spec{URL: "https://h/o/r", Path: "p", Ref: "abc"}},
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
		{"https://user:pass@h/o/r", "credentials"},
		{"https://user:pass@h/o/r//p", "credentials"},
		{"ssh://git@h/o/r", "ssh"},
		{"file:///tmp/repo", "file"},
		{"github:/r", "owner"},
		{"github:o/", "repo"},
		{"github:o", "repo"},
		{"https://h//p", "owner"},
		{"https://../../x", "host"},
		{"https://../o/r", "host"},
		{"github:../x", "owner"},
		{"github:o/..", "repo"},
		{"github:o/r%2f", "repo"},
		{"https://-h/o/r", "host"},
		{"https://h/-o/r", "owner"},
		{"https://h:/o/r", "host"},
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

func TestCanonicalURL(t *testing.T) {
	for _, in := range []string{
		"github:o/r",
		"github:o/r.git",
		"https://github.com/o/r.git",
		"https://GitHub.com/o/r",
	} {
		s, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if s.URL != "https://github.com/o/r" {
			t.Errorf("Parse(%q).URL = %q", in, s.URL)
		}
		if s.Key() != "github.com/o/r" {
			t.Errorf("Parse(%q).Key() = %q", in, s.Key())
		}
	}
}

func TestCredentialsNotEchoed(t *testing.T) {
	inputs := []string{
		"https://user:tok@github.com/o/r",
		"ssh://git:tok@h/o/r",
		"file://user:tok@x/r",
		"user:tok@host/o/r",
		"https://user:pa//ss@h/o/r",
		"github:user:tok@host/o/r",
		"user:tok@h://x",
		"https://user:tok@h/o/r//../x",
	}
	for _, allow := range []bool{false, true} {
		for _, in := range inputs {
			_, err := ParseWith(in, Options{AllowFile: allow})
			if err == nil {
				t.Errorf("ParseWith(%q, allowFile=%v) expected error", in, allow)
				continue
			}
			for _, leak := range []string{"tok", "pa//ss", "user:"} {
				if strings.Contains(err.Error(), leak) {
					t.Errorf("ParseWith(%q, allowFile=%v) error %q leaks %q", in, allow, err, leak)
				}
			}
		}
	}
}

func TestExtraSegmentNamed(t *testing.T) {
	_, err := Parse("github:o/r/x")
	if err == nil || !strings.Contains(err.Error(), `"x"`) {
		t.Errorf("Parse(github:o/r/x) error = %v, want it to name \"x\"", err)
	}
}

func TestRefAfterRepoStillParses(t *testing.T) {
	for in, want := range map[string]Spec{
		"github:o/r@main":    {URL: "https://github.com/o/r", Ref: "main"},
		"https://h/o/r@main": {URL: "https://h/o/r", Ref: "main"},
	} {
		got, err := Parse(in)
		if err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", in, got, err, want)
		}
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

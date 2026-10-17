package apm

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/manifest"
)

const local = "github:rojcarranza/agent-skills"

func parseFile(t *testing.T, localSource string) Result {
	t.Helper()
	f, err := os.Open("testdata/apm.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	r, err := Parse(f, localSource)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestParseMapsSkills(t *testing.T) {
	r := parseFile(t, local)
	if len(r.Skills) != 18 {
		t.Fatalf("got %d skills, want 18: %v", len(r.Skills), r.Skills)
	}
	want := map[string]manifest.Skill{
		"roj-code-review-pr": {Source: local + "//skills/roj-code-review-pr"},
		"code-tour": {Source: "github:github/awesome-copilot//skills/code-tour",
			Ref: "7cce7cfb4b61196c36d7e8eb8475ae84b356b126"},
		"write-pr-description": {Source: "github:warpdotdev/common-skills//.agents/skills/write-pr-description",
			Ref: "2a03b403b4f8d42cb54b8953697737193b2e8306"},
		"caveman": {Source: "https://github.com/juliusbrussee/caveman//skills/caveman",
			Ref: "2e08b9177c07bb7249a8a2d1a6758e5db281d002", Targets: []string{"copilot"}},
	}
	for n, w := range want {
		if g := r.Skills[n]; !reflect.DeepEqual(g, w) {
			t.Errorf("%s = %+v, want %+v", n, g, w)
		}
	}
	if !reflect.DeepEqual(r.Targets, []string{"claude", "copilot"}) {
		t.Errorf("targets = %v", r.Targets)
	}
	if n := len(parseFile(t, "").Skills); n != 7 {
		t.Errorf("without local source got %d skills, want 7", n)
	}
}

func TestParseSkipsMCPAndInstructionsWithWarning(t *testing.T) {
	r := parseFile(t, "")
	for _, w := range []string{
		"mcp server io.github.github/github-mcp-server is not managed by cali",
		"local skill ./skills/roj-soundboarding needs --local-source",
		"includes is ignored",
	} {
		if !contains(r.Skipped, w) {
			t.Errorf("Skipped lacks %q: %v", w, r.Skipped)
		}
	}
	if len(r.Skipped) != 13 { // 11 local + 1 mcp + includes
		t.Errorf("len(Skipped) = %d: %v", len(r.Skipped), r.Skipped)
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

func TestParseErrors(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"duplicate": {"dependencies:\n  apm:\n    - a/b/x/s#1\n    - c/d/y/s#2\n", "duplicate install name s"},
		"shape":     {"dependencies:\n  apm:\n    - ./skills/a\n    - [1]\n", "entry 1"},
		"badsource": {"dependencies:\n  apm:\n    - git: ftp://h/o/r.git\n      path: p/x\n", "invalid source spec"},
		"badname":   {"dependencies:\n  apm:\n    - a/b/p/Bad#1\n", "Bad"},
	} {
		_, err := Parse(strings.NewReader(tc.in), local)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

func TestParseLocalSourceWithRef(t *testing.T) {
	r, err := Parse(strings.NewReader("dependencies:\n  apm:\n    - ./skills/a\n"), local+"@v1")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := r.Skills["a"], (manifest.Skill{Source: local + "//skills/a", Ref: "v1"}); got.Source != want.Source || got.Ref != want.Ref {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

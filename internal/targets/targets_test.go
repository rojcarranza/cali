package targets

import "testing"

func TestDirTable(t *testing.T) {
	cases := []struct {
		target string
		scope  Scope
		want   string
	}{
		{"claude", Project, "/p/.claude/skills"},
		{"claude", Global, "/h/.claude/skills"},
		{"copilot", Project, "/p/.github/skills"},
		{"copilot", Global, "/h/.copilot/skills"},
	}
	for _, c := range cases {
		got, err := Dir(c.target, c.scope, "/p", "/h")
		if err != nil {
			t.Fatalf("%s/%d: %v", c.target, c.scope, err)
		}
		if got != c.want {
			t.Errorf("%s/%d: got %q, want %q", c.target, c.scope, got, c.want)
		}
	}
}

func TestDirUnknown(t *testing.T) {
	if _, err := Dir("cursor", Project, "/p", "/h"); err == nil {
		t.Fatal("want error for unknown target")
	}
	if _, err := Dir("claude", Scope(9), "/p", "/h"); err == nil {
		t.Fatal("want error for unknown scope")
	}
}

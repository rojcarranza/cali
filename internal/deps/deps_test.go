package deps

import (
	"reflect"
	"testing"

	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
)

func TestRequiresUnion(t *testing.T) {
	fm := skillfs.Frontmatter{Metadata: map[string]string{"cali-requires": " b, a ,,c "}}
	got := Requires(manifest.Skill{Requires: []string{"c", "d"}}, fm)
	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRequiresEmptyMetadata(t *testing.T) {
	if got := Requires(manifest.Skill{}, skillfs.Frontmatter{}); len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
	got := Requires(manifest.Skill{Requires: []string{"z"}}, skillfs.Frontmatter{})
	if want := []string{"z"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCheckSatisfiedByGlobal(t *testing.T) {
	// the caller merges project and global names per target
	installed := map[string][]string{"claude": {"a", "dep"}}
	if w := Check(installed, map[string][]string{"a": {"dep"}}); len(w) != 0 {
		t.Fatalf("got %v, want none", w)
	}
}

func TestCheckMissingReportedPerTarget(t *testing.T) {
	installed := map[string][]string{"claude": {"a", "dep"}, "copilot": {"a"}}
	got := Check(installed, map[string][]string{"a": {"dep"}})
	want := []Warning{{Skill: "a", Missing: "dep", Target: "copilot"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

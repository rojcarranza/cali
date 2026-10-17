package status

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/deps"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

const (
	treeA   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	treeB   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	commitA = "1111111111111111111111111111111111111111"
	commitB = "2222222222222222222222222222222222222222"
)

type fakeFetcher struct {
	commit, tree string
	err          error
	calls        []source.Spec
}

func (f *fakeFetcher) Latest(s source.Spec) (string, string, error) {
	f.calls = append(f.calls, s)
	return f.commit, f.tree, f.err
}

func skillTree(name string) skillfs.Tree {
	return skillfs.Tree{{Path: "SKILL.md", Data: []byte("---\nname: " + name + "\ndescription: D.\n---\nbody\n")}}
}

// fixture installs skill x (tree treeA, pristine stored) for target claude in a project.
func fixture(t *testing.T) (Input, *fakeFetcher, *bytes.Buffer) {
	t.Helper()
	root, home, data := t.TempDir(), t.TempDir(), t.TempDir()
	if err := store.Put(data, treeA, skillTree("x")); err != nil {
		t.Fatal(err)
	}
	if err := skillTree("x").WriteTo(filepath.Join(root, ".claude", "skills", "x")); err != nil {
		t.Fatal(err)
	}
	f := &fakeFetcher{commit: commitA, tree: treeA}
	warn := &bytes.Buffer{}
	return Input{
		Scope: targets.Project, Root: root, Home: home, DataDir: data,
		Manifest: manifest.Manifest{Version: 1, Targets: []string{"claude"}, Skills: map[string]manifest.Skill{
			"x": {Source: "https://github.com/o/r//skills/x"},
		}},
		Lock: lock.Lock{Version: 1, Skills: map[string]lock.Entry{
			"x": {Source: "https://github.com/o/r", Path: "skills/x", Commit: commitA, Tree: treeA},
		}},
		Fetcher: f, Warn: warn,
	}, f, warn
}

func compute(t *testing.T, in Input) ([]Row, []deps.Warning) {
	t.Helper()
	rows, warns, err := Compute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	return rows, warns
}

func states(t *testing.T, rows []Row, skill, target string) []State {
	t.Helper()
	for _, r := range rows {
		if r.Skill == skill && r.Target == target && !slices.Contains(r.States, Shadowed) {
			return r.States
		}
	}
	t.Fatalf("no row %s/%s in %+v", skill, target, rows)
	return nil
}

func want(t *testing.T, got []State, w ...State) {
	t.Helper()
	if !slices.Equal(got, w) {
		t.Fatalf("states = %v, want %v", got, w)
	}
}

func TestStatusOK(t *testing.T) {
	in, _, _ := fixture(t)
	rows, warns := compute(t, in)
	if len(rows) != 1 || len(warns) != 0 {
		t.Fatalf("rows=%+v warns=%+v", rows, warns)
	}
	want(t, states(t, rows, "x", "claude"), OK)
}

func TestStatusModified(t *testing.T) {
	in, _, _ := fixture(t)
	p := filepath.Join(in.Root, ".claude", "skills", "x", "SKILL.md")
	if err := os.WriteFile(p, []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), Modified)

	// An added file counts as an edit too.
	in2, _, _ := fixture(t)
	if err := os.WriteFile(filepath.Join(in2.Root, ".claude", "skills", "x", "extra.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, _ = compute(t, in2)
	want(t, states(t, rows, "x", "claude"), Modified)
}

func TestStatusOutdatedOnlyWhenSubtreeChanged(t *testing.T) {
	in, f, _ := fixture(t)
	f.commit = commitB // newer commit elsewhere in the repo, same subtree
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), OK)
	if len(f.calls) != 1 || f.calls[0].URL != "https://github.com/o/r" || f.calls[0].Path != "skills/x" {
		t.Fatalf("calls = %+v", f.calls)
	}

	f.tree = treeB
	rows, _ = compute(t, in)
	want(t, states(t, rows, "x", "claude"), Outdated)
}

func TestStatusPinnedCommitNeverOutdated(t *testing.T) {
	in, f, _ := fixture(t)
	in.Manifest.Skills["x"] = manifest.Skill{Source: "https://github.com/o/r//skills/x", Ref: commitA}
	f.tree = treeB
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), OK)
	if len(f.calls) != 0 {
		t.Fatalf("pinned skill was fetched: %+v", f.calls)
	}
}

func TestStatusMissing(t *testing.T) {
	in, _, _ := fixture(t)
	if err := os.RemoveAll(filepath.Join(in.Root, ".claude", "skills", "x")); err != nil {
		t.Fatal(err)
	}
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), Missing)
}

func TestStatusUnmanaged(t *testing.T) {
	in, _, _ := fixture(t)
	skills := filepath.Join(in.Root, ".claude", "skills")
	for _, d := range []string{"z", ".cali-staging-123"} {
		if err := skillTree(d).WriteTo(filepath.Join(skills, d)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(skills, "no-skill-md"), 0o755); err != nil {
		t.Fatal(err)
	}
	rows, _ := compute(t, in)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	want(t, states(t, rows, "x", "claude"), OK)
	want(t, states(t, rows, "z", "claude"), Unmanaged)
}

func TestStatusBaseLost(t *testing.T) {
	in, _, _ := fixture(t)
	if err := os.RemoveAll(filepath.Join(in.DataDir, "pristine", treeA)); err != nil {
		t.Fatal(err)
	}
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), BaseLost)
	if rows[0].Detail != "run cali sync" {
		t.Fatalf("detail = %q", rows[0].Detail)
	}
}

func TestStatusShadowed(t *testing.T) {
	in, _, _ := fixture(t)
	in.Global = manifest.Manifest{Version: 1, Targets: []string{"claude", "copilot"}, Skills: map[string]manifest.Skill{
		"x": {Source: "https://github.com/o/r//skills/x"},
		"y": {Source: "https://github.com/o/r//skills/y"},
	}}
	rows, _ := compute(t, in)
	// Project x row, then the shadowed global x for claude only (project x is not in copilot); y is not shown.
	if len(rows) != 2 || rows[1].Skill != "x" || rows[1].Target != "claude" ||
		!slices.Equal(rows[1].States, []State{Shadowed}) || rows[1].Detail != "global" {
		t.Fatalf("rows = %+v", rows)
	}
	want(t, rows[0].States, OK)
}

func TestStatusOfflineSkipsFetch(t *testing.T) {
	in, f, _ := fixture(t)
	in.Offline = true
	f.tree = treeB
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), OK)
	if len(f.calls) != 0 {
		t.Fatalf("offline fetched: %+v", f.calls)
	}
}

func TestStatusFetchErrorIsWarningNotFailure(t *testing.T) {
	in, f, warn := fixture(t)
	in.Manifest.Skills["w"] = manifest.Skill{Source: "https://github.com/o/r//skills/w"}
	in.Lock.Skills["w"] = lock.Entry{Source: "https://github.com/o/r", Path: "skills/w", Commit: commitA, Tree: treeA}
	if err := skillTree("x").WriteTo(filepath.Join(in.Root, ".claude", "skills", "w")); err != nil {
		t.Fatal(err)
	}
	f.err = errors.New("network down")
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), OK)
	if got := warn.String(); got != "warning: https://github.com/o/r: network down\n" {
		t.Fatalf("warning = %q (want it once per source)", got)
	}
}

func TestStatusReportsMissingDependency(t *testing.T) {
	in, _, _ := fixture(t)
	in.Manifest.Skills["x"] = manifest.Skill{Source: "https://github.com/o/r//skills/x", Requires: []string{"y", "g"}}
	// g is installed globally for claude, which satisfies the dependency.
	in.Global = manifest.Manifest{Version: 1, Targets: []string{"claude"}, Skills: map[string]manifest.Skill{
		"g": {Source: "https://github.com/o/r//skills/g"},
	}}
	if err := skillTree("g").WriteTo(filepath.Join(in.Home, ".claude", "skills", "g")); err != nil {
		t.Fatal(err)
	}
	_, warns := compute(t, in)
	if !slices.Equal(warns, []deps.Warning{{Skill: "x", Missing: "y", Target: "claude"}}) {
		t.Fatalf("warns = %+v", warns)
	}

	// cali-requires in the installed SKILL.md counts as well.
	in2, _, _ := fixture(t)
	p := filepath.Join(in2.Root, ".claude", "skills", "x", "SKILL.md")
	if err := os.WriteFile(p, []byte("---\nname: x\ndescription: D.\nmetadata:\n  cali-requires: \"q\"\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, warns = compute(t, in2)
	if len(warns) != 1 || warns[0].Missing != "q" || !strings.Contains(warns[0].Skill, "x") {
		t.Fatalf("warns = %+v", warns)
	}
}

// With no manifest targets both targets are scanned, so the fixture's folder x shows as unmanaged.
func TestStatusNoManifestOnlyUnmanaged(t *testing.T) {
	in, _, _ := fixture(t)
	in.Manifest = manifest.Manifest{Version: 1, Skills: map[string]manifest.Skill{}}
	in.Lock = lock.Lock{Version: 1, Skills: map[string]lock.Entry{}}
	rows, warns := compute(t, in)
	if len(rows) != 1 || rows[0].Skill != "x" || rows[0].Target != "claude" || !slices.Equal(rows[0].States, []State{Unmanaged}) || len(warns) != 0 {
		t.Fatalf("rows=%+v warns=%+v", rows, warns)
	}
}

func TestStatusRefInSource(t *testing.T) {
	in, f, _ := fixture(t)
	in.Manifest.Skills["x"] = manifest.Skill{Source: "https://github.com/o/r//skills/x@" + commitA}
	f.tree = treeB
	rows, _ := compute(t, in)
	want(t, states(t, rows, "x", "claude"), OK)
	if len(f.calls) != 0 {
		t.Fatalf("skill pinned by @sha was fetched: %+v", f.calls)
	}

	in.Manifest.Skills["x"] = manifest.Skill{Source: "https://github.com/o/r//skills/x@v1.0.0"}
	compute(t, in)
	if len(f.calls) != 1 || f.calls[0].Ref != "v1.0.0" {
		t.Fatalf("calls = %+v, want one for v1.0.0", f.calls)
	}
}

func TestStatusOddManagedFolderIsARow(t *testing.T) {
	for _, tc := range []struct{ name, detail string }{
		{"symlink", "symlink, not managed by Cali"},
		{"file", "not a directory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, _, _ := fixture(t)
			in.Manifest.Skills["y"] = manifest.Skill{Source: "https://github.com/o/r//skills/y"}
			final := filepath.Join(in.Root, ".claude", "skills", "x")
			if err := os.RemoveAll(final); err != nil {
				t.Fatal(err)
			}
			if tc.name == "symlink" {
				if err := os.Symlink(t.TempDir(), final); err != nil {
					t.Skip("symlinks unavailable:", err)
				}
			} else if err := os.WriteFile(final, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			rows, _, err := Compute(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			want(t, states(t, rows, "x", "claude"), Modified)
			want(t, states(t, rows, "y", "claude"), Missing)
			for _, r := range rows {
				if r.Skill == "x" && r.Detail != tc.detail {
					t.Fatalf("detail = %q, want %q", r.Detail, tc.detail)
				}
			}
		})
	}
}

func TestStatusUnreadableManagedFolderIsARow(t *testing.T) {
	in, _, _ := fixture(t)
	in.Manifest.Skills["y"] = manifest.Skill{Source: "https://github.com/o/r//skills/y"}
	if err := os.Symlink(t.TempDir(), filepath.Join(in.Root, ".claude", "skills", "x", "notes")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	rows, _, err := Compute(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	want(t, states(t, rows, "x", "claude"), Modified)
	want(t, states(t, rows, "y", "claude"), Missing)
	for _, r := range rows {
		if r.Skill == "x" && r.Detail != "skill contains a symlink: notes" {
			t.Fatalf("detail = %q", r.Detail)
		}
	}
}

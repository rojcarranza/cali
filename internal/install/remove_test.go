package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/skillfs"
)

func backups(t *testing.T, e env, name string) []string {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(e.deps.Dirs.Data, "backups", name))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, d := range ents {
		out = append(out, filepath.Join(e.deps.Dirs.Data, "backups", name, d.Name()))
	}
	return out
}

func TestRemoveBacksUpAndDropsEntries(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	for _, s := range []string{"x", "y"} {
		if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/" + s}); err != nil {
			t.Fatal(err)
		}
	}
	if err := Remove(context.Background(), e.deps, "nope", false); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("err = %v, want not-in-manifest error", err)
	}
	want := readTree(t, filepath.Join(r, "skills", "x"))

	var out bytes.Buffer
	e.deps.Stdout = &out
	if err := Remove(context.Background(), e.deps, "x", false); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".claude/skills/x", ".github/skills/x"} {
		if exists(filepath.Join(e.project, filepath.FromSlash(d))) {
			t.Errorf("%s not removed", d)
		}
	}
	if !exists(filepath.Join(e.project, ".claude", "skills", "y", "SKILL.md")) {
		t.Fatal("other skill touched")
	}
	bs := backups(t, e, "x")
	if len(bs) != 2 {
		t.Fatalf("backups = %v, want one per target", bs)
	}
	for _, b := range bs {
		if !skillfs.Equal(readTree(t, filepath.Join(b, "files")), want) {
			t.Errorf("backup %s differs from the removed folder", b)
		}
		if !strings.Contains(out.String(), b) {
			t.Errorf("output %q does not name backup %s", out.String(), b)
		}
	}
	m := readManifest(t, filepath.Join(e.project, "cali.yaml"))
	l, _ := lock.Read(filepath.Join(e.project, "cali.lock"))
	if _, ok := m.Skills["x"]; ok {
		t.Fatal("manifest entry kept")
	}
	if _, ok := l.Skills["x"]; ok {
		t.Fatal("lock entry kept")
	}
	if _, ok := m.Skills["y"]; !ok {
		t.Fatal("other manifest entry dropped")
	}
	if _, ok := l.Skills["y"]; !ok {
		t.Fatal("other lock entry dropped")
	}
}

func TestRemoveBlocksOnLocalEditsWithoutForce(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x"}); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(e.project, ".claude", "skills", "x")
	copilot := filepath.Join(e.project, ".github", "skills", "x")
	write(t, copilot, "SKILL.md", "my edit\n")

	err := Remove(context.Background(), e.deps, "x", false)
	if err == nil || !strings.Contains(err.Error(), copilot) || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want local-edits error naming %s", err, copilot)
	}
	if !exists(claude) || !exists(copilot) || backups(t, e, "x") != nil {
		t.Fatal("something changed before the refusal")
	}
	if _, ok := readManifest(t, filepath.Join(e.project, "cali.yaml")).Skills["x"]; !ok {
		t.Fatal("manifest entry dropped on refusal")
	}

	if err := Remove(context.Background(), e.deps, "x", true); err != nil {
		t.Fatal(err)
	}
	if exists(claude) || exists(copilot) {
		t.Fatal("--force did not remove the folders")
	}
	edited := false
	for _, b := range backups(t, e, "x") {
		if bb, err := os.ReadFile(filepath.Join(b, "files", "SKILL.md")); err == nil && string(bb) == "my edit\n" {
			edited = true
		}
	}
	if !edited {
		t.Fatal("the local edit is not in a backup")
	}
}

func TestRemoveKeepsPristineWhileReferenced(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	for _, g := range []bool{false, true} {
		if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Global: g}); err != nil {
			t.Fatal(err)
		}
	}
	l, _ := lock.Read(filepath.Join(e.project, "cali.lock"))
	pristine := filepath.Join(e.deps.Dirs.Data, "pristine", l.Skills["x"].Tree)
	if err := Remove(context.Background(), e.deps, "x", false); err != nil {
		t.Fatal(err)
	}
	if !exists(pristine) {
		t.Fatal("pristine copy deleted while the global lock still references it")
	}
	if !exists(filepath.Join(e.deps.Dirs.Home, ".claude", "skills", "x", "SKILL.md")) {
		t.Fatal("global install touched by a project remove")
	}
}

func TestRemoveRefusesSymlinkedFolder(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(e.project, ".claude", "skills", "x")
	elsewhere := t.TempDir()
	if err := os.Rename(final, filepath.Join(elsewhere, "x")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(elsewhere, "x"), final); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := Remove(context.Background(), e.deps, "x", true); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v, want symlink refusal", err)
	}
	if !exists(filepath.Join(elsewhere, "x", "SKILL.md")) {
		t.Fatal("symlink target touched")
	}
}

func TestRemoveAbortsWhenFolderChangesDuringBackup(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(e.project, ".claude", "skills", "x")
	afterBackup = func(dir string) { write(t, dir, "SKILL.md", "late edit\n") }
	t.Cleanup(func() { afterBackup = nil })

	err := Remove(context.Background(), e.deps, "x", false)
	if err == nil || !strings.Contains(err.Error(), final) {
		t.Fatalf("err = %v, want abort naming %s", err, final)
	}
	if b, _ := os.ReadFile(filepath.Join(final, "SKILL.md")); string(b) != "late edit\n" {
		t.Fatal("folder removed or changed after a concurrent edit")
	}
	if _, ok := readManifest(t, filepath.Join(e.project, "cali.yaml")).Skills["x"]; !ok {
		t.Fatal("manifest entry dropped")
	}
	if l, _ := lock.Read(filepath.Join(e.project, "cali.lock")); l.Skills["x"].Tree == "" {
		t.Fatal("lock entry dropped")
	}
}

func TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(e.project, ".claude", "skills", "x")
	crlf := strings.ReplaceAll(readFile(t, filepath.Join(final, "SKILL.md")), "\n", "\r\n")
	afterBackup = func(dir string) { write(t, dir, "SKILL.md", crlf) }
	t.Cleanup(func() { afterBackup = nil })

	if err := Remove(context.Background(), e.deps, "x", false); err == nil || !strings.Contains(err.Error(), final) {
		t.Fatalf("err = %v, want abort naming %s", err, final)
	}
	if got := readFile(t, filepath.Join(final, "SKILL.md")); got != crlf {
		t.Fatal("folder removed after a line-ending-only edit")
	}
}

package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/store"
)

func addX(t *testing.T, e env, r string) {
	t.Helper()
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, p string, m manifest.Manifest) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteWith(p, m, source.Options{AllowFile: true}); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hasAction(rep Report, kind, name, target, detail string) bool {
	for _, a := range rep.Actions {
		if a.Kind == kind && a.Name == name && a.Target == target && a.Detail == detail {
			return true
		}
	}
	return false
}

func TestSyncFromLockOnFreshMachine(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	addX(t, e, r)
	want := readTree(t, filepath.Join(r, "skills", "x"))
	lockPath := filepath.Join(e.project, "cali.lock")
	lockBefore := readFile(t, lockPath)
	// Upstream moves on; sync must install the locked commit, not the new head.
	write(t, r, "skills/x/SKILL.md", skillMD("x")+"newer\n")
	git(t, r, "commit", "-q", "-am", "two")
	// Fresh machine: no data, no cache, no installed folders.
	for _, d := range []string{e.deps.Dirs.Data, e.deps.Dirs.Cache, filepath.Join(e.project, ".claude")} {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	e.deps.Stdout = &out
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(e.project, ".claude", "skills", "x")
	if !skillfs.Equal(readTree(t, installed), want) {
		t.Fatal("installed folder differs from the locked tree")
	}
	l, err := lock.Read(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Get(e.deps.Dirs.Data, l.Skills["x"].Tree); err != nil || !ok {
		t.Fatalf("pristine not rebuilt: ok=%v err=%v", ok, err)
	}
	if readFile(t, lockPath) != lockBefore {
		t.Fatal("lock changed")
	}
	if !hasAction(rep, "installed", "x", "claude", "") || !strings.Contains(out.String(), "installed x (claude)\n") {
		t.Fatalf("report = %+v, out = %q", rep, out.String())
	}

	// A second sync has nothing to do.
	out.Reset()
	if rep, err := Sync(context.Background(), e.deps, true); err != nil || len(rep.Actions) != 0 || out.Len() != 0 {
		t.Fatalf("second sync: rep=%+v err=%v out=%q", rep, err, out.String())
	}
}

func TestSyncFrozenFailsWhenLockNeedsChange(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	addX(t, e, r)
	mPath, lockPath := filepath.Join(e.project, "cali.yaml"), filepath.Join(e.project, "cali.lock")

	// A manifest entry missing from the lock.
	m := readManifest(t, mPath)
	m.Skills["y"] = manifest.Skill{Source: fileURL(r) + "//skills/y"}
	writeManifest(t, mPath, m)
	lockBefore := readFile(t, lockPath)
	_, err := Sync(context.Background(), e.deps, true)
	if err == nil || !strings.Contains(err.Error(), "y") || !strings.Contains(err.Error(), "--frozen") {
		t.Fatalf("err = %v, want frozen error naming y", err)
	}
	if readFile(t, lockPath) != lockBefore || exists(filepath.Join(e.project, ".claude", "skills", "y")) {
		t.Fatal("frozen sync wrote something")
	}

	// A lock entry no longer in the manifest.
	delete(m.Skills, "y")
	delete(m.Skills, "x")
	writeManifest(t, mPath, m)
	_, err = Sync(context.Background(), e.deps, true)
	if err == nil || !strings.Contains(err.Error(), "x") {
		t.Fatalf("err = %v, want frozen error naming x", err)
	}
	if readFile(t, lockPath) != lockBefore {
		t.Fatal("frozen sync changed the lock")
	}

	// Without --frozen the stale entry is dropped and the folder kept.
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	l, _ := lock.Read(lockPath)
	if _, ok := l.Skills["x"]; ok {
		t.Fatal("stale lock entry kept")
	}
	if !exists(filepath.Join(e.project, ".claude", "skills", "x", "SKILL.md")) {
		t.Fatal("sync deleted a folder")
	}
}

func TestSyncNeverOverwritesModified(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	addX(t, e, r)
	md := filepath.Join(e.project, ".claude", "skills", "x", "SKILL.md")
	write(t, filepath.Dir(md), "SKILL.md", "my edit\n")
	var out bytes.Buffer
	e.deps.Stdout = &out
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, md) != "my edit\n" {
		t.Fatal("modified folder overwritten")
	}
	if !hasAction(rep, "kept", "x", "claude", "modified") || out.String() != "kept x (claude): modified\n" {
		t.Fatalf("report = %+v, out = %q", rep, out.String())
	}
}

func TestSyncNeverOverwritesUnmanaged(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	mPath := filepath.Join(e.project, "cali.yaml")
	writeManifest(t, mPath, manifest.Manifest{Version: 1, Targets: []string{"claude"}, Skills: map[string]manifest.Skill{
		"x": {Source: fileURL(r) + "//skills/x"},
		"y": {Source: fileURL(r) + "//skills/y"},
	}})
	skills := filepath.Join(e.project, ".claude", "skills")
	write(t, filepath.Join(skills, "x"), "SKILL.md", "mine\n")
	write(t, filepath.Join(skills, "y"), "SKILL.md", skillMD("y")) // identical to upstream

	var out bytes.Buffer
	e.deps.Stdout = &out
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(skills, "x", "SKILL.md")) != "mine\n" {
		t.Fatal("unmanaged folder overwritten")
	}
	if !hasAction(rep, "kept", "x", "claude", "unmanaged") || !hasAction(rep, "adopted", "y", "claude", "") {
		t.Fatalf("report = %+v", rep)
	}
	if out.String() != "kept x (claude): unmanaged\nadopted y (claude)\n" {
		t.Fatalf("out = %q", out.String())
	}
	l, _ := lock.Read(filepath.Join(e.project, "cali.lock"))
	if _, ok := l.Skills["x"]; ok {
		t.Fatal("an unmanaged, differing folder must not be locked")
	}
	if _, ok := l.Skills["y"]; !ok {
		t.Fatal("identical folder not adopted into the lock")
	}
}

func TestSyncMissingEntryResolvesRefAndWritesLock(t *testing.T) {
	r := monorepo(t)
	git(t, r, "tag", "v1")
	v1, v1tree := git(t, r, "rev-parse", "HEAD"), git(t, r, "rev-parse", "HEAD:skills/x")
	want := readTree(t, filepath.Join(r, "skills", "x"))
	write(t, r, "skills/x/SKILL.md", skillMD("x")+"newer\n")
	git(t, r, "commit", "-q", "-am", "two")

	e := newEnv(t)
	mPath := filepath.Join(e.project, "cali.yaml")
	writeManifest(t, mPath, manifest.Manifest{Version: 1, Targets: []string{"claude"}, Skills: map[string]manifest.Skill{
		"x": {Source: fileURL(r) + "//skills/x", Ref: "v1"},
	}})
	mBefore := readFile(t, mPath)
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	l, err := lock.Read(filepath.Join(e.project, "cali.lock"))
	if err != nil {
		t.Fatal(err)
	}
	ent := l.Skills["x"]
	if ent != (lock.Entry{Source: fileURL(r), Path: "skills/x", Ref: "v1", Commit: v1, Tree: v1tree}) {
		t.Fatalf("lock entry = %+v", ent)
	}
	if !skillfs.Equal(readTree(t, filepath.Join(e.project, ".claude", "skills", "x")), want) {
		t.Fatal("installed folder is not the v1 tree")
	}
	if !hasAction(rep, "installed", "x", "claude", "") {
		t.Fatalf("report = %+v", rep)
	}
	if readFile(t, mPath) != mBefore {
		t.Fatal("manifest rewritten although unchanged")
	}
}

func TestSyncReportsBaseLost(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	addX(t, e, r)
	// Rewrite history so the locked commit is gone upstream, then lose the local copies.
	git(t, r, "commit", "-q", "--amend", "-m", "rewritten")
	// The mirror is a partial clone that fetches missing objects on demand, so the old commit must really be gone.
	git(t, r, "reflog", "expire", "--expire=now", "--all")
	git(t, r, "gc", "-q", "--prune=now")
	for _, d := range []string{e.deps.Dirs.Data, e.deps.Dirs.Cache, filepath.Join(e.project, ".claude")} {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	e.deps.Stdout = &out
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAction(rep, "base-lost", "x", "", "") || out.String() != "base-lost x\n" {
		t.Fatalf("report = %+v, out = %q", rep, out.String())
	}
}

func TestSyncNothingToDo(t *testing.T) {
	e := newEnv(t)
	rep, err := Sync(context.Background(), e.deps, true)
	if err != nil || len(rep.Actions) != 0 {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if exists(filepath.Join(e.project, "cali.lock")) || exists(filepath.Join(e.project, "cali.yaml")) {
		t.Fatal("sync with nothing to do wrote files")
	}
}

func editLock(t *testing.T, p string, f func(*lock.Entry)) {
	t.Helper()
	l, err := lock.Read(p)
	if err != nil {
		t.Fatal(err)
	}
	e := l.Skills["x"]
	f(&e)
	l.Skills["x"] = e
	if err := lock.Write(p, l); err != nil {
		t.Fatal(err)
	}
}

func TestSyncIgnoresLockEntryFromAnotherSource(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	addX(t, e, r)
	lockPath := filepath.Join(e.project, "cali.lock")
	// A committed lock pointing elsewhere; the fixture URL does not exist, so fetching it would fail.
	editLock(t, lockPath, func(en *lock.Entry) { en.Source = fileURL(filepath.Join(t.TempDir(), "elsewhere")) })
	for _, d := range []string{e.deps.Dirs.Data, e.deps.Dirs.Cache} {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	l, _ := lock.Read(lockPath)
	if l.Skills["x"].Source != fileURL(r) {
		t.Fatalf("lock entry not re-resolved from the manifest: %+v", l.Skills["x"])
	}
}

func TestSyncFrozenFailsOnLockDrift(t *testing.T) {
	cases := map[string]func(*lock.Entry){
		"source": func(en *lock.Entry) { en.Source = "file:///nonexistent/elsewhere" },
		"path":   func(en *lock.Entry) { en.Path = "skills/y" },
		"ref":    func(en *lock.Entry) { en.Ref = "v9" },
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			r := monorepo(t)
			e := newEnv(t)
			addX(t, e, r)
			lockPath := filepath.Join(e.project, "cali.lock")
			editLock(t, lockPath, f)
			before := readFile(t, lockPath)
			_, err := Sync(context.Background(), e.deps, true)
			if err == nil || !strings.Contains(err.Error(), "--frozen") || !strings.Contains(err.Error(), ": x") {
				t.Fatalf("err = %v, want frozen error naming x", err)
			}
			if readFile(t, lockPath) != before {
				t.Fatal("frozen sync changed the lock")
			}
		})
	}
}

func TestSyncUnmanagedTargetBlocksOtherTargets(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	writeManifest(t, filepath.Join(e.project, "cali.yaml"), manifest.Manifest{Version: 1, Targets: []string{"claude", "copilot"}, Skills: map[string]manifest.Skill{
		"x": {Source: fileURL(r) + "//skills/x"},
		"y": {Source: fileURL(r) + "//skills/y"},
	}})
	write(t, filepath.Join(e.project, ".claude", "skills", "x"), "SKILL.md", "mine\n")
	write(t, filepath.Join(e.project, ".claude", "skills", "y"), "SKILL.md", "mine\n")
	write(t, filepath.Join(e.project, ".github", "skills", "y"), "SKILL.md", skillMD("y")) // identical
	var out bytes.Buffer
	e.deps.Stdout = &out
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	if out.String() != "kept x (claude): unmanaged\nkept y (claude): unmanaged\n" {
		t.Fatalf("out = %q", out.String())
	}
	if exists(filepath.Join(e.project, ".github", "skills", "x")) {
		t.Fatal("installed into another target of a skill with an unmanaged target")
	}
}

func TestSyncRefusesLockWithoutManifest(t *testing.T) {
	r := monorepo(t)
	e := newEnv(t)
	addX(t, e, r)
	manifestPath := filepath.Join(e.project, "cali.yaml")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(e.project, "cali.lock")
	before := readFile(t, lockPath)
	_, err := Sync(context.Background(), e.deps, false)
	if err == nil || !strings.Contains(err.Error(), manifestPath) {
		t.Fatalf("err = %v, want an error naming %s", err, manifestPath)
	}
	if got := readFile(t, lockPath); got != before {
		t.Fatalf("lock changed:\n%s", got)
	}
}

func TestSyncKeepsLockWhenOnlyRefChanged(t *testing.T) {
	r := monorepo(t)
	git(t, r, "tag", "v1")
	write(t, r, "skills/x/SKILL.md", skillMD("x")+"newer\n")
	git(t, r, "commit", "-q", "-am", "two")
	git(t, r, "tag", "v2")

	e := newEnv(t)
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: fileURL(r) + "//skills/x@v1", Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	mPath, lockPath := filepath.Join(e.project, "cali.yaml"), filepath.Join(e.project, "cali.lock")
	folder := filepath.Join(e.project, ".claude", "skills", "x")
	m := readManifest(t, mPath)
	s := m.Skills["x"]
	s.Ref = "v2"
	m.Skills["x"] = s
	writeManifest(t, mPath, m)
	lockBefore, folderBefore := readFile(t, lockPath), readTree(t, folder)

	if _, err := Sync(context.Background(), e.deps, true); err == nil || !strings.Contains(err.Error(), "x") || !strings.Contains(err.Error(), "cali update") {
		t.Fatalf("frozen err = %v, want error naming x and cali update", err)
	}
	var out bytes.Buffer
	e.deps.Stdout = &out
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	if out.String() != "ref changed x: run cali update\n" {
		t.Fatalf("out = %q", out.String())
	}
	if readFile(t, lockPath) != lockBefore {
		t.Fatal("sync changed the lock entry")
	}
	if !skillfs.Equal(readTree(t, folder), folderBefore) {
		t.Fatal("sync touched the folder")
	}
}

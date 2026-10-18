package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
)

// selfRepo makes e.project a git repo that holds skills/x and a cali.yaml
// naming it as a same-repo source, like a skills repo that ships its manifest.
func selfRepo(t *testing.T) env {
	t.Helper()
	isolate(t)
	e := newEnv(t)
	git(t, e.project, "init", "-q", "-b", "main")
	write(t, e.project, "skills/x/SKILL.md", skillMD("x"))
	writeManifest(t, filepath.Join(e.project, "cali.yaml"), manifest.Manifest{
		Version: 1, Targets: []string{"claude"},
		Skills: map[string]manifest.Skill{"x": {Source: ".//skills/x"}},
	})
	git(t, e.project, "add", ".")
	git(t, e.project, "commit", "-q", "-m", "one")
	return e
}

func readLock(t *testing.T, e env) lock.Lock {
	t.Helper()
	l, err := lock.Read(filepath.Join(e.project, "cali.lock"))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestSyncSameRepoLocksTreeWithoutCommit(t *testing.T) {
	e := selfRepo(t)
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	want := lock.Entry{Source: ".", Path: "skills/x", Tree: git(t, e.project, "rev-parse", "HEAD:skills/x")}
	if got := readLock(t, e).Skills["x"]; got != want {
		t.Fatalf("lock entry = %+v, want %+v", got, want)
	}
	if !exists(filepath.Join(e.project, ".claude", "skills", "x", "SKILL.md")) {
		t.Fatal("x not installed")
	}

	// A new skill and its lock entry land in one change: sync locks it from
	// the commit checked out, before any merge.
	write(t, e.project, "skills/z/SKILL.md", skillMD("z"))
	m := readManifest(t, filepath.Join(e.project, "cali.yaml"))
	m.Skills["z"] = manifest.Skill{Source: ".//skills/z"}
	writeManifest(t, filepath.Join(e.project, "cali.yaml"), m)
	git(t, e.project, "add", "skills", "cali.yaml")
	git(t, e.project, "commit", "-q", "-m", "add z")
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAction(rep, "installed", "z", "claude", "") || readLock(t, e).Skills["z"].Tree == "" {
		t.Fatalf("z not installed and locked: %+v", rep)
	}
	if _, err := Sync(context.Background(), e.deps, true); err != nil {
		t.Fatalf("frozen sync after locking: %v", err)
	}
}

func TestSyncSameRepoLockBehind(t *testing.T) {
	e := selfRepo(t)
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	lockBefore := readFile(t, filepath.Join(e.project, "cali.lock"))
	write(t, e.project, "skills/x/SKILL.md", skillMD("x")+"newer\n")
	git(t, e.project, "commit", "-q", "-am", "change x")

	if _, err := Sync(context.Background(), e.deps, true); err == nil || !strings.Contains(err.Error(), "behind the repo, run cali update: x") {
		t.Fatalf("frozen sync with a stale lock: err = %v", err)
	}
	rep, err := Sync(context.Background(), e.deps, false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAction(rep, "outdated", "x", "", "run cali update") {
		t.Fatalf("report = %+v", rep)
	}
	if readFile(t, filepath.Join(e.project, "cali.lock")) != lockBefore {
		t.Fatal("sync moved the lock; that is update's job")
	}
}

func TestSyncSameRepoRebuildsPristineFromTree(t *testing.T) {
	e := selfRepo(t)
	if _, err := Sync(context.Background(), e.deps, false); err != nil {
		t.Fatal(err)
	}
	// Fresh machine: the clone and its lock, nothing else.
	for _, d := range []string{e.deps.Dirs.Data, e.deps.Dirs.Cache, filepath.Join(e.project, ".claude")} {
		if err := os.RemoveAll(d); err != nil {
			t.Fatal(err)
		}
	}
	rep, err := Sync(context.Background(), e.deps, true)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAction(rep, "installed", "x", "claude", "") {
		t.Fatalf("report = %+v", rep)
	}
}

func TestSyncSameRepoNeedsGitRepo(t *testing.T) {
	e := newEnv(t)
	writeManifest(t, filepath.Join(e.project, "cali.yaml"), manifest.Manifest{
		Version: 1, Targets: []string{"claude"},
		Skills: map[string]manifest.Skill{"x": {Source: ".//skills/x"}},
	})
	if _, err := Sync(context.Background(), e.deps, false); err == nil || !strings.Contains(err.Error(), "inside a git repository") {
		t.Fatalf("err = %v", err)
	}
}

func TestAddSameRepo(t *testing.T) {
	e := selfRepo(t)
	write(t, e.project, "skills/w/SKILL.md", skillMD("w"))
	git(t, e.project, "add", ".")
	git(t, e.project, "commit", "-q", "-m", "add w")
	if _, err := Add(context.Background(), e.deps, AddRequest{Spec: ".//skills/w"}); err != nil {
		t.Fatal(err)
	}
	if got := readManifest(t, filepath.Join(e.project, "cali.yaml")).Skills["w"].Source; got != ".//skills/w" {
		t.Fatalf("manifest source = %q", got)
	}
	if e := readLock(t, e).Skills["w"]; e.Source != "." || e.Commit != "" || e.Tree == "" {
		t.Fatalf("lock entry = %+v", e)
	}
}

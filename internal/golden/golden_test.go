// Package golden asserts the brief's metric: the same cali.lock produces
// byte-identical skill folders on macOS, Windows and WSL.
package golden

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/dirs"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/skillfs"
)

// testdata/fixture.bundle was built once, in a scratch dir outside the repo:
//
//	export GIT_CONFIG_GLOBAL=/dev/null GIT_AUTHOR_NAME=Fixture GIT_AUTHOR_EMAIL=fixture@example.invalid \
//	  GIT_COMMITTER_NAME=Fixture GIT_COMMITTER_EMAIL=fixture@example.invalid \
//	  GIT_AUTHOR_DATE=2026-01-01T00:00:00Z GIT_COMMITTER_DATE=2026-01-01T00:00:00Z
//	git init -q -b main . && git config core.autocrlf false && mkdir -p skills/demo/sub
//	printf -- '---\nname: demo\ndescription: golden fixture\n---\r\ncrlf line one\r\ncrlf line two\r\n' > skills/demo/SKILL.md
//	printf 'lf line one\nlf line two\n' > skills/demo/notes.txt
//	printf 'bin\000\001\002\377\000end' > skills/demo/data.bin
//	printf '#!/bin/sh\necho hi\n' > skills/demo/run.sh && chmod 755 skills/demo/run.sh
//	printf 'nested\n' > skills/demo/sub/inner.txt
//	git add -A && git commit -q -m fixture && git bundle create fixture.bundle main
//
// Commit b362669a582e2b5dcf5c2ffdbf814651628eee43. Rebuilding changes the lock
// and the hashes below. testdata/cali.lock holds the @FIXTURE@ placeholder where
// the per-run bare repo URL goes.
//
// wantWindows is wantUnix's hash with every exec flag forced to "-" (the exec
// bit is ignored on Windows); it was computed on macOS.
const (
	wantUnix    = "87a17119cb986f17fecea354232c36252edf86992a84b740aafb58b4ae713b64"
	wantWindows = "fe8477dc2a768540422a5e0a06681567fb4c7e565fbd9be2d6e72866ccad93cd"
)

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}

// treeHash is SHA-256 over "path\0x|-\0hex(sha256(bytes))\n" per file, sorted by path.
func treeHash(tr skillfs.Tree) string {
	h := sha256.New()
	for _, f := range tr { // ReadDir sorts by slash path
		x := "-"
		if f.Exec && runtime.GOOS != "windows" {
			x = "x"
		}
		sum := sha256.Sum256(f.Data)
		h.Write([]byte(f.Path + "\x00" + x + "\x00" + hex.EncodeToString(sum[:]) + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func TestSameLockSameBytes(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	if err := os.WriteFile(os.Getenv("GIT_CONFIG_GLOBAL"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	bare := filepath.Join(base, "fixture.git")
	run(t, "git", "clone", "-q", "--bare", filepath.Join("testdata", "fixture.bundle"), bare)
	run(t, "git", "--git-dir", bare, "symbolic-ref", "HEAD", "refs/heads/main") // a bundle clone leaves HEAD unset

	project := filepath.Join(base, "project")
	home := filepath.Join(base, "home")
	for _, d := range []string{project, home} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	url := "file://" + filepath.ToSlash(bare)
	lockTmpl, err := os.ReadFile(filepath.Join("testdata", "cali.lock"))
	if err != nil {
		t.Fatal(err)
	}
	lock := bytes.ReplaceAll(lockTmpl, []byte("@FIXTURE@"), []byte(url))
	manifest := "version: 1\ntargets: [claude]\nskills:\n  demo:\n    source: " + url + "//skills/demo\n    ref: main\n"
	for name, data := range map[string]string{"cali.yaml": manifest, "cali.lock": string(lock)} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	d := install.Deps{
		Dirs: dirs.Dirs{
			Config: filepath.Join(base, "config"),
			Data:   filepath.Join(base, "data"),
			Cache:  filepath.Join(base, "cache"),
			Home:   home,
		},
		Getenv:          func(string) string { return "" },
		GhToken:         func() (string, error) { return "", errors.New("no gh") },
		AllowFileScheme: true,
		Cwd:             project,
	}
	if _, err := install.Sync(context.Background(), d, true); err != nil {
		t.Fatal(err)
	}

	installed := filepath.Join(project, ".claude", "skills", "demo")
	tr, err := skillfs.ReadDir(installed)
	if err != nil {
		t.Fatal(err)
	}
	crlf, err := os.ReadFile(filepath.Join(installed, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(crlf), "\r\n") {
		t.Fatal("SKILL.md lost its CRLF line endings")
	}
	want := wantUnix
	if runtime.GOOS == "windows" {
		want = wantWindows
	}
	if got := treeHash(tr); got != want {
		t.Fatalf("tree hash = %s, want %s", got, want)
	}
}

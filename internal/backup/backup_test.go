package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rojcarranza/cali/internal/skillfs"
)

func fixedNow(t *testing.T) {
	t.Helper()
	old := now
	now = func() time.Time { return time.Date(2026, 10, 16, 9, 8, 7, 0, time.FixedZone("x", 2*3600)) }
	t.Cleanup(func() { now = old })
}

var tree = skillfs.Tree{
	{Path: "SKILL.md", Data: []byte("---\nname: x\n---\n")},
	{Path: "scripts/run.sh", Exec: true, Data: []byte("echo\n")},
}

func TestSaveWritesFilesAndPatch(t *testing.T) {
	fixedNow(t)
	data := t.TempDir()
	p, err := Save(data, "x", tree, []byte("diff\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(data, "backups", "x", "20261016T070807Z") // UTC
	if p != want {
		t.Fatalf("path = %s, want %s", p, want)
	}
	got, err := skillfs.ReadDir(filepath.Join(p, "files"))
	if err != nil || !skillfs.Equal(got, tree) {
		t.Fatalf("files differ: err=%v", err)
	}
	if b, err := os.ReadFile(filepath.Join(p, "local.patch")); err != nil || string(b) != "diff\n" {
		t.Fatalf("local.patch = %q, %v", b, err)
	}
}

func TestSaveWithoutPatchWritesNoPatchFile(t *testing.T) {
	fixedNow(t)
	p, err := Save(t.TempDir(), "x", tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(p, "local.patch")); !os.IsNotExist(err) {
		t.Fatalf("local.patch should not exist: %v", err)
	}
}

func TestSaveAppendsSuffixOnCollision(t *testing.T) {
	fixedNow(t)
	data := t.TempDir()
	var paths []string
	for i := 0; i < 3; i++ {
		p, err := Save(data, "x", tree, nil)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Base(p))
	}
	want := []string{"20261016T070807Z", "20261016T070807Z-1", "20261016T070807Z-2"}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %v, want %v", paths, want)
		}
	}
}

func TestSaveRejectsBadName(t *testing.T) {
	data := t.TempDir()
	for _, n := range []string{"", "../x", "X", "a/b", "con"} {
		if _, err := Save(data, n, tree, nil); err == nil {
			t.Errorf("Save(%q) succeeded", n)
		}
	}
	if _, err := os.Lstat(filepath.Join(data, "backups")); !os.IsNotExist(err) {
		t.Fatal("nothing may be written for a bad name")
	}
}

func TestSaveDirsArePrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes only")
	}
	fixedNow(t)
	data := t.TempDir()
	p, err := Save(data, "x", tree, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{filepath.Join(data, "backups"), filepath.Join(data, "backups", "x"), p} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %v, want 0700", d, fi.Mode().Perm())
		}
	}
}

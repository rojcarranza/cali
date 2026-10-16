package skillfs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func skillMD() File { return File{Path: "SKILL.md", Data: []byte("# s\n")} }

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink on this OS: %v", err)
	}
}

func TestEqualIgnoresCRLFInText(t *testing.T) {
	a := Tree{{Path: "SKILL.md", Data: []byte("a\r\nb\r\n")}}
	b := Tree{{Path: "SKILL.md", Data: []byte("a\nb\n")}}
	if !Equal(a, b) {
		t.Fatal("text files differing only in CRLF must be equal")
	}
	if d := Diff(a, b); len(d) != 0 {
		t.Fatalf("Diff = %v, want none", d)
	}
}

func TestEqualBinaryExact(t *testing.T) {
	a := Tree{{Path: "x.bin", Data: []byte("a\x00\r\nb")}}
	b := Tree{{Path: "x.bin", Data: []byte("a\x00\nb")}}
	if Equal(a, b) {
		t.Fatal("binary files must be compared byte for byte")
	}
	if !Equal(a, a) {
		t.Fatal("identical binary must be equal")
	}
	// NUL after the first 8000 bytes does not make a file binary.
	late := append([]byte(strings.Repeat("a", 8000)), 0)
	c := Tree{{Path: "l.txt", Data: append(append([]byte{}, late...), []byte("\r\n")...)}}
	d := Tree{{Path: "l.txt", Data: append(append([]byte{}, late...), []byte("\n")...)}}
	if !Equal(c, d) {
		t.Fatal("NUL past byte 8000 must still count as text")
	}
}

func TestEqualComparesExecAndFileSet(t *testing.T) {
	a := Tree{skillMD()}
	b := Tree{{Path: "SKILL.md", Data: []byte("# s\n"), Exec: true}}
	if Equal(a, b) {
		t.Fatal("Exec must be compared")
	}
	if Equal(a, Tree{skillMD(), {Path: "extra", Data: nil}}) {
		t.Fatal("file set must be compared")
	}
}

func static(fs ...File) func(*testing.T) (Tree, error) {
	return func(*testing.T) (Tree, error) { return Tree(fs), nil }
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name string
		tree func(t *testing.T) (Tree, error)
	}{
		{"symlink", func(t *testing.T) (Tree, error) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, "SKILL.md", filepath.Join(dir, "link"))
			return ReadDir(dir)
		}},
		{"dotdot", static(skillMD(), File{Path: "../x"})},
		{"dotdot-mid", static(skillMD(), File{Path: "a/../../x"})},
		{"absolute", static(skillMD(), File{Path: "/etc/passwd"})},
		{"backslash", static(skillMD(), File{Path: `a\b`})},
		{"drive", static(skillMD(), File{Path: "C:/x"})},
		{"reserved", static(skillMD(), File{Path: "NUL.md"})},
		{"reserved-lower-dir", static(skillMD(), File{Path: "com1/x"})},
		{"colon", static(skillMD(), File{Path: "a:b"})},
		{"star", static(skillMD(), File{Path: "a*b"})},
		{"control", static(skillMD(), File{Path: "a\x01b"})},
		{"trailing-dot", static(skillMD(), File{Path: "a."})},
		{"trailing-space", static(skillMD(), File{Path: "dir /f"})},
		{"duplicate", static(skillMD(), skillMD())},
		{"empty-segment", static(skillMD(), File{Path: "a//b"})},
		{"no-skill-md", static(File{Path: "README.md"})},
		{"nested-skill-md-only", static(File{Path: "sub/SKILL.md"})},
		{"empty-tree", static()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, err := tc.tree(t)
			if err == nil {
				err = Validate(tr)
			}
			if err == nil {
				t.Fatalf("expected rejection for %v", tr)
			}
		})
	}
	if err := Validate(Tree{skillMD(), {Path: "scripts/run.sh", Exec: true}}); err != nil {
		t.Fatalf("valid tree rejected: %v", err)
	}
}

func TestReadDirRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../SKILL.md", filepath.Join(dir, "sub", "evil"))
	_, err := ReadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "sub/evil") {
		t.Fatalf("err = %v, want one naming sub/evil", err)
	}
}

func TestWriteToThenReadDirEqual(t *testing.T) {
	tr := Tree{
		{Path: "SKILL.md", Data: []byte("# s\r\n")},
		{Path: "bin/data.bin", Data: []byte{0, 1, 2}},
		{Path: "scripts/run.sh", Data: []byte("#!/bin/sh\n"), Exec: true},
	}
	dir := filepath.Join(t.TempDir(), "out")
	if err := tr.WriteTo(dir); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(tr, got) {
		t.Fatalf("round trip differs: %v", Diff(tr, got))
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(dir, "scripts", "run.sh"))
		if st.Mode().Perm() != 0o755 {
			t.Fatalf("exec mode = %v", st.Mode().Perm())
		}
		st, _ = os.Stat(filepath.Join(dir, "SKILL.md"))
		if st.Mode().Perm() != 0o644 {
			t.Fatalf("file mode = %v", st.Mode().Perm())
		}
	}
}

func TestWriteToInvalidWritesNothing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	err := Tree{skillMD(), {Path: "../escape"}}.WriteTo(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if _, serr := os.Lstat(dir); !os.IsNotExist(serr) {
		t.Fatalf("dir must not be created, stat err = %v", serr)
	}
}

func TestWriteToRefusesSymlinkComponent(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "out")
	outside := filepath.Join(base, "outside")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, outside, filepath.Join(dir, "sub"))
	err := Tree{skillMD(), {Path: "sub/f.txt", Data: []byte("pwn")}}.WriteTo(dir)
	if err == nil {
		t.Fatal("expected refusal")
	}
	if _, serr := os.Lstat(filepath.Join(outside, "f.txt")); !os.IsNotExist(serr) {
		t.Fatal("wrote through symlink")
	}
}

func TestWriteToRefusesSymlinkFile(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "out")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "victim")
	if err := os.WriteFile(target, []byte("orig"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, target, filepath.Join(dir, "SKILL.md"))
	if err := (Tree{skillMD()}).WriteTo(dir); err == nil {
		t.Fatal("expected refusal")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "orig" {
		t.Fatal("victim overwritten")
	}
}

func TestDiffKinds(t *testing.T) {
	base := Tree{
		{Path: "SKILL.md", Data: []byte("a\n")},
		{Path: "gone.txt", Data: []byte("g")},
		{Path: "same.txt", Data: []byte("s")},
		{Path: "x.sh", Data: []byte("x")},
	}
	other := Tree{
		{Path: "SKILL.md", Data: []byte("b\n")},
		{Path: "new.txt", Data: []byte("n")},
		{Path: "same.txt", Data: []byte("s")},
		{Path: "x.sh", Data: []byte("x"), Exec: true},
	}
	want := []Change{
		{"SKILL.md", "modified"},
		{"gone.txt", "removed"},
		{"new.txt", "added"},
		{"x.sh", "modified"},
	}
	got := Diff(base, other)
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

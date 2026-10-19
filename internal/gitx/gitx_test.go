package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
)

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, rel, data string, mode os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

// fixture builds a repo with skills/a and skills/b and returns its dir. The
// committed SKILL.md of skill a contains CRLF line endings.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "core.autocrlf", "false")
	write(t, dir, "skills/a/SKILL.md", "---\nname: a\n---\r\nline\r\n", 0o644)
	write(t, dir, "skills/a/scripts/run.sh", "#!/bin/sh\n", 0o755)
	write(t, dir, "skills/b/SKILL.md", "b\n", 0o644)
	git(t, dir, "add", ".")
	// Windows git does not record the OS file mode; set the bit in the index.
	git(t, dir, "update-index", "--chmod=+x", "skills/a/scripts/run.sh")
	git(t, dir, "commit", "-q", "-m", "one")
	// From here on the author's clone has autocrlf=true, as on Windows.
	git(t, dir, "config", "core.autocrlf", "true")
	// Run everything after the commit under a global autocrlf=true, as on
	// Windows; Export must still return the committed bytes.
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, []byte("[core]\n\tautocrlf = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	return dir
}

func fixtureSpec(t *testing.T, repoDir string) source.Spec {
	t.Helper()
	s, err := source.ParseWith("file://"+filepath.ToSlash(repoDir), source.Options{AllowFile: true})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func openFetched(t *testing.T, repoDir string) *Repo {
	t.Helper()
	r, err := Open(t.TempDir(), fixtureSpec(t, repoDir))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Fetch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestExportEqualsCommittedBytes(t *testing.T) {
	dir := fixture(t)
	r := openFetched(t, dir)
	commit, err := r.Resolve("")
	if err != nil || !sha40.MatchString(commit) {
		t.Fatalf("Resolve = %q, %v", commit, err)
	}
	if !r.HasCommit(commit) || r.HasCommit(strings.Repeat("0", 40)) {
		t.Fatal("HasCommit wrong")
	}
	tr, err := exportAt(r, commit, "skills/a")
	if err != nil {
		t.Fatal(err)
	}
	var md, sh *skillfs.File
	for i := range tr {
		switch tr[i].Path {
		case "SKILL.md":
			md = &tr[i]
		case "scripts/run.sh":
			sh = &tr[i]
		}
	}
	if md == nil || sh == nil || len(tr) != 2 {
		t.Fatalf("tree = %+v", tr)
	}
	if string(md.Data) != "---\nname: a\n---\r\nline\r\n" {
		t.Fatalf("SKILL.md bytes changed: %q", md.Data)
	}
	if string(sh.Data) != "#!/bin/sh\n" || !sh.Exec {
		t.Fatalf("run.sh = %+v", sh)
	}
	if md.Exec {
		t.Fatal("SKILL.md must not be exec")
	}
}

func TestResolveRefs(t *testing.T) {
	dir := fixture(t)
	first := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "tag", "v1")
	git(t, dir, "checkout", "-q", "-b", "dev")
	write(t, dir, "skills/b/SKILL.md", "b2\n", 0o644)
	git(t, dir, "commit", "-q", "-am", "two")
	dev := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", "main")

	r := openFetched(t, dir)
	for ref, want := range map[string]string{"": first, "v1": first, "dev": dev, "main": first, dev: dev} {
		got, err := r.Resolve(ref)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", ref, got, err, want)
		}
	}
	if _, err := r.Resolve("nope"); err == nil {
		t.Error("want error for unknown ref")
	}
	if _, err := r.Resolve("--upload-pack=x"); err == nil {
		t.Error("want error for option-like ref")
	}
}

func TestTreeSHAChangesOnlyWhenSubtreeChanges(t *testing.T) {
	dir := fixture(t)
	r := openFetched(t, dir)
	c1, _ := r.Resolve("")
	a1, err := r.TreeSHA(c1, "skills/a")
	if err != nil || !sha40.MatchString(a1) {
		t.Fatalf("TreeSHA = %q, %v", a1, err)
	}
	write(t, dir, "skills/b/SKILL.md", "changed\n", 0o644)
	git(t, dir, "commit", "-q", "-am", "touch b")
	if err := r.Fetch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	c2, _ := r.Resolve("")
	if c1 == c2 {
		t.Fatal("commit should have moved")
	}
	a2, _ := r.TreeSHA(c2, "skills/a")
	b1, _ := r.TreeSHA(c1, "skills/b")
	b2, _ := r.TreeSHA(c2, "skills/b")
	if a1 != a2 {
		t.Errorf("skills/a tree changed: %s -> %s", a1, a2)
	}
	if b1 == b2 {
		t.Error("skills/b tree should change")
	}
	if _, err := r.TreeSHA(c2, "skills/missing"); err == nil {
		t.Error("want error for missing path")
	}
}

func TestExportRejectsSymlinkAndSubmodule(t *testing.T) {
	dir := fixture(t)
	// Commit a symlink without needing OS symlink support.
	linkBlob := hashString(t, dir, "SKILL.md")
	git(t, dir, "update-index", "--add", "--cacheinfo", "120000,"+linkBlob+",skills/b/link")
	git(t, dir, "commit", "-q", "-m", "link")
	r := openFetched(t, dir)
	c, _ := r.Resolve("")
	_, err := exportAt(r, c, "skills/b")
	if err == nil || !strings.Contains(err.Error(), "link") || !errors.Is(err, skillfs.ErrInvalid) {
		t.Fatalf("symlink err = %v", err)
	}

	git(t, dir, "rm", "-q", "--cached", "skills/b/link")
	git(t, dir, "update-index", "--add", "--cacheinfo", "160000,0123456789abcdef0123456789abcdef01234567,skills/b/sub")
	git(t, dir, "commit", "-q", "-m", "sub")
	if err := r.Fetch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	c, _ = r.Resolve("")
	_, err = exportAt(r, c, "skills/b")
	if err == nil || !strings.Contains(err.Error(), "sub") || !errors.Is(err, skillfs.ErrInvalid) {
		t.Fatalf("submodule err = %v", err)
	}
}

func TestFilteredFetchIsBlobless(t *testing.T) {
	dir := fixture(t)
	git(t, dir, "config", "uploadpack.allowFilter", "true")
	git(t, dir, "config", "uploadpack.allowAnySHA1InWant", "true")
	r := openFetched(t, dir)
	out, err := r.git(context.Background(), nil, "rev-list", "--objects", "--missing=print", "--all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains("\n"+string(out), "\n?") {
		t.Fatalf("fetch was not blob-less, no missing objects:\n%s", out)
	}
	c, _ := r.Resolve("")
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	tr, err := exportAt(r, c, "skills/a")
	if err != nil || len(tr) != 2 {
		t.Fatalf("Export over lazy fetch = %v, %v", tr, err)
	}
	// Both missing blobs come down in one fetch, not one fetch per blob.
	log, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(log), "built-in: git fetch"); n != 1 {
		t.Errorf("Export ran %d fetches, want 1:\n%s", n, log)
	}
	if string(tr[0].Data) != "---\nname: a\n---\r\nline\r\n" {
		t.Fatalf("bytes = %q", tr[0].Data)
	}
}

func TestOpenRefusesEscapingKey(t *testing.T) {
	cache := t.TempDir()
	_, err := Open(cache, source.Spec{URL: "https://../../x"})
	if err == nil {
		t.Fatal("want error for a key that escapes the cache")
	}
	if _, serr := os.Stat(filepath.Join(cache, "x.git")); serr == nil {
		t.Fatal("repository was created outside the cache")
	}
}

func TestExportEnforcesLimitsBeforeReading(t *testing.T) {
	dir := fixture(t)
	r := openFetched(t, dir)
	c, _ := r.Resolve("")
	old := limits
	t.Cleanup(func() { limits = old })

	limits.files = 1
	if _, err := exportAt(r, c, "skills/a"); err == nil || !strings.Contains(err.Error(), "files") || !errors.Is(err, skillfs.ErrInvalid) {
		t.Fatalf("file count err = %v", err)
	}
	limits.files, limits.file = old.files, 5
	if _, err := exportAt(r, c, "skills/a"); err == nil || !strings.Contains(err.Error(), "limit is 5") || !errors.Is(err, skillfs.ErrInvalid) {
		t.Fatalf("file size err = %v", err)
	}
	limits.file, limits.tree = old.file, 10
	if _, err := exportAt(r, c, "skills/a"); err == nil || !strings.Contains(err.Error(), "larger than") || !errors.Is(err, skillfs.ErrInvalid) {
		t.Fatalf("tree size err = %v", err)
	}
}

func TestResolveRejectsRevisionExpressions(t *testing.T) {
	r := openFetched(t, fixture(t))
	for _, ref := range []string{"main~1", "main^", "main^{tree}", "main..main", "@{u}", "main:skills", "a\\b"} {
		if _, err := r.Resolve(ref); err == nil || !strings.Contains(err.Error(), "invalid ref") {
			t.Errorf("Resolve(%q) err = %v, want invalid ref", ref, err)
		}
	}
}

func TestEnvironStripsInheritedConfig(t *testing.T) {
	base := []string{
		"PATH=/bin", "GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=a", "GIT_CONFIG_VALUE_0=b",
		"GIT_CONFIG_KEY_1=c", "GIT_CONFIG_VALUE_1=d", "GIT_ALLOW_PROTOCOL=ext",
		"GIT_DIR=/other/.git", "GIT_WORK_TREE=/other", "GIT_INDEX_FILE=/other/.git/index",
	}
	got := environ(base, "https", AuthEnv("github.com", func(string) string { return "t" }, nil))
	m := envMap(got)
	if m["GIT_CONFIG_COUNT"] != "1" || m["GIT_CONFIG_KEY_0"] != "http.https://github.com/.extraheader" ||
		m["GIT_ALLOW_PROTOCOL"] != "https" || m["GIT_TERMINAL_PROMPT"] != "0" || m["PATH"] != "/bin" {
		t.Fatalf("env = %v", got)
	}
	if _, ok := m["GIT_CONFIG_KEY_1"]; ok {
		t.Fatalf("inherited key survived: %v", got)
	}
	if _, ok := m["GIT_CONFIG_VALUE_1"]; ok {
		t.Fatalf("inherited value survived: %v", got)
	}
	for _, e := range got {
		if e == "GIT_CONFIG_COUNT=2" || e == "GIT_ALLOW_PROTOCOL=ext" || strings.HasPrefix(e, "GIT_DIR=") ||
			strings.HasPrefix(e, "GIT_WORK_TREE=") || strings.HasPrefix(e, "GIT_INDEX_FILE=") {
			t.Fatalf("inherited entry survived: %v", got)
		}
	}
}

func TestSanitizeStderr(t *testing.T) {
	in := "fatal: \x1b[31mred\x1b]0;title\x07 \u009b2J bell\x07 nul\x00 line\nnext\x7f"
	out := sanitize(in, nil)
	for _, r := range out {
		if r < ' ' || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Fatalf("control rune %U left in %q", r, out)
		}
	}
	if !strings.Contains(out, "line next") {
		t.Fatalf("out = %q", out)
	}
	long := sanitize(strings.Repeat("x", 10000), nil)
	if len(long) > maxStderr+3 {
		t.Fatalf("len = %d, want capped", len(long))
	}
}

func TestSymlinkErrorQuotesPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip(`a path containing " is illegal on Windows`)
	}
	// Paths in errors are %q-quoted so a hostile name cannot inject text.
	dir := fixture(t)
	linkBlob := hashString(t, dir, "x")
	git(t, dir, "update-index", "--add", "--cacheinfo", "120000,"+linkBlob+",skills/b/ev\"il")
	git(t, dir, "commit", "-q", "-m", "evil")
	r := openFetched(t, dir)
	c, _ := r.Resolve("")
	_, err := exportAt(r, c, "skills/b")
	if err == nil || !strings.Contains(err.Error(), `"ev\"il"`) {
		t.Fatalf("err = %v", err)
	}
}

func hashString(t *testing.T, dir, s string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(f, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return git(t, dir, "hash-object", "-w", f)
}

func TestFetchErrorDoesNotLeakToken(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	r, err := Open(t.TempDir(), fixtureSpec(t, missing))
	if err != nil {
		t.Fatal(err)
	}
	getenv := func(string) string { return "s3cr3t-token" }
	err = r.Fetch(context.Background(), AuthEnv("github.com", getenv, nil))
	if err == nil {
		t.Fatal("want fetch error")
	}
	if strings.Contains(err.Error(), "s3cr3t-token") {
		t.Fatalf("token leaked: %v", err)
	}
}

func TestListSkillDirs(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "SKILL.md", "root\n", 0o644)
	write(t, dir, "skills/a/SKILL.md", "a\n", 0o644)
	write(t, dir, "skills/a/nested/SKILL.md", "n\n", 0o644)
	write(t, dir, "x/skill.md", "lowercase name is not a skill\n", 0o644)
	write(t, dir, "d1/d2/d3/d4/SKILL.md", "depth 4\n", 0o644)
	write(t, dir, "d1/d2/d3/d4/d5/SKILL.md", "depth 5\n", 0o644)
	write(t, dir, "notes/SKILL.md/readme", "a dir named SKILL.md\n", 0o644)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	r := openFetched(t, dir)
	commit, err := r.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ListSkillDirs(commit, 4)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"", "d1/d2/d3/d4", "skills/a", "skills/a/nested"}
	if strings.Join(got, ",") != strings.Join(want, ",") || len(got) != len(want) {
		t.Fatalf("ListSkillDirs = %q, want %q", got, want)
	}
	if _, err := r.ListSkillDirs("HEAD", 4); err == nil {
		t.Fatal("want error for a non-SHA commit")
	}
}

func TestReadSkillFiles(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "SKILL.md", "root\n", 0o644)
	for i := 0; i < 20; i++ {
		write(t, dir, fmt.Sprintf("skills/s%02d/SKILL.md", i), fmt.Sprintf("s%02d\n", i), 0o644)
	}
	write(t, dir, "skills/big/SKILL.md", "far too big\n", 0o644)
	write(t, dir, "d1/d2/d3/d4/d5/SKILL.md", "too deep\n", 0o644)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "one")
	r := openFetched(t, dir)
	commit, err := r.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	old := limits.file
	limits.file = 5
	defer func() { limits.file = old }()
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	got, err := r.ReadSkillFiles(commit, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 21 || string(got[""]) != "root\n" || string(got["skills/s07"]) != "s07\n" {
		t.Fatalf("ReadSkillFiles = %d files, root %q, s07 %q", len(got), got[""], got["skills/s07"])
	}
	if _, ok := got["skills/big"]; ok {
		t.Fatal("oversize SKILL.md: want it skipped")
	}
	// A fixed number of git processes, not one or two per skill.
	log, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(log), "built-in: git "); n > 4 {
		t.Errorf("ReadSkillFiles ran %d git commands, want at most 4:\n%s", n, log)
	}
	if _, err := r.ReadSkillFiles("HEAD", 4); err == nil {
		t.Fatal("want error for a non-SHA commit")
	}
}

func TestReadFile(t *testing.T) {
	dir := fixture(t)
	r := openFetched(t, dir)
	commit, err := r.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.ReadFile(commit, "skills/b/SKILL.md")
	if err != nil || string(b) != "b\n" {
		t.Fatalf("ReadFile = %q, %v", b, err)
	}
	if _, err := r.ReadFile(commit, "skills/nope/SKILL.md"); err == nil {
		t.Fatal("missing file: want error")
	}
	if _, err := r.ReadFile(commit, "skills/a"); err == nil {
		t.Fatal("directory: want error")
	}
	old := limits.file
	limits.file = 1
	defer func() { limits.file = old }()
	if _, err := r.ReadFile(commit, "skills/b/SKILL.md"); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversize: err = %v", err)
	}
}

// exportAt exports path at commit, as callers do: TreeSHA, then Export.
func exportAt(r *Repo, commit, path string) (skillfs.Tree, error) {
	tree, err := r.TreeSHA(commit, path)
	if err != nil {
		return nil, err
	}
	return r.Export(tree)
}

func TestOpenWritesConfigOnlyWhenNeeded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory does not stop writes on windows")
	}
	cache, spec := t.TempDir(), fixtureSpec(t, fixture(t))
	r, err := Open(cache, spec)
	if err != nil {
		t.Fatal(err)
	}
	// A configured mirror needs no write: Open must succeed with the dir read-only.
	if err := os.Chmod(r.Dir, 0o555); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(cache, spec); err != nil {
		t.Fatalf("second Open wrote config: %v", err)
	}
	if err := os.Chmod(r.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A missing or changed value is written back.
	git(t, r.Dir, "config", "--unset", "remote.origin.promisor")
	git(t, r.Dir, "config", "remote.origin.url", "https://example.invalid/x")
	if _, err := Open(cache, spec); err != nil {
		t.Fatal(err)
	}
	if got := git(t, r.Dir, "config", "remote.origin.promisor"); got != "true" {
		t.Errorf("promisor = %q", got)
	}
	if got := git(t, r.Dir, "config", "remote.origin.url"); got != spec.URL {
		t.Errorf("url = %q, want %q", got, spec.URL)
	}
}

func TestFetchFallsBackOnlyWhenTheFilterIsRejected(t *testing.T) {
	src := fixture(t)
	git(t, src, "config", "uploadpack.allowFilter", "true")
	git(t, src, "config", "uploadpackfilter.allow", "false")
	r, err := Open(t.TempDir(), fixtureSpec(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Fetch(context.Background(), nil); err != nil {
		t.Fatalf("no fallback to a full fetch: %v", err)
	}
	if _, err := r.Resolve("main"); err != nil {
		t.Fatal(err)
	}

	// Any other failure is returned as is, without a second, full fetch.
	gone, err := Open(t.TempDir(), fixtureSpec(t, filepath.Join(t.TempDir(), "missing")))
	if err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "trace")
	t.Setenv("GIT_TRACE", trace)
	if err := gone.Fetch(context.Background(), nil); err == nil {
		t.Fatal("fetch of a missing repo succeeded")
	}
	log, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(log), "--no-filter") {
		t.Errorf("fell back to a full fetch:\n%s", log)
	}
}

func TestOpenLocalReadsCheckedOutHead(t *testing.T) {
	dir := fixture(t)
	// A linked worktree on another branch: its HEAD, not main's, is the one read.
	wt := filepath.Join(t.TempDir(), "wt")
	git(t, dir, "worktree", "add", "-q", "-b", "feature", wt)
	write(t, wt, "skills/c/SKILL.md", "c\n", 0o644)
	git(t, wt, "add", ".")
	git(t, wt, "commit", "-q", "-m", "two")

	r, err := OpenLocal(filepath.Join(wt, "skills"))
	if err != nil {
		t.Fatal(err)
	}
	commit, err := r.Resolve("")
	if err != nil || commit != git(t, wt, "rev-parse", "HEAD") {
		t.Fatalf("Resolve(\"\") = %q, %v; want the worktree HEAD", commit, err)
	}
	tree, err := r.TreeSHA(commit, "skills/c")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Export(tree); err != nil {
		t.Fatal(err)
	}

	// An inherited GIT_DIR (as in a git hook of another repo) is ignored.
	t.Setenv("GIT_DIR", filepath.Join(dir, ".git"))
	if r, err := OpenLocal(wt); err != nil || r.Dir == filepath.Join(dir, ".git") {
		t.Fatalf("OpenLocal under GIT_DIR: dir=%v err=%v", r, err)
	}
	t.Setenv("GIT_DIR", "")
	if _, err := OpenLocal(t.TempDir()); err == nil || !strings.Contains(err.Error(), "inside a git repository") || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("OpenLocal outside a repo: err = %v", err)
	}
}

package update

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/merge"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/status"
	"github.com/rojcarranza/cali/internal/store"
	"github.com/rojcarranza/cali/internal/targets"
)

// apply plans with o and applies; out collects what Apply prints. Every
// apply also checks that no installed file holds a conflict marker.
func (f fx) apply(t *testing.T, o Options, pr Prompter) (string, error) {
	t.Helper()
	var out bytes.Buffer
	f.d.Stdout = &out
	o.Apply = true
	p, err := Plan(context.Background(), f.d, o)
	if err != nil {
		t.Fatal(err)
	}
	err = Apply(context.Background(), f.d, p, pr)
	f.noMarkers(t)
	return out.String(), err
}

func (f fx) noMarkers(t *testing.T) {
	t.Helper()
	for _, root := range []string{f.d.Cwd, f.d.Dirs.Home} {
		_ = filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil || !e.Type().IsRegular() {
				return nil
			}
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("<<<<<<<")) {
				t.Errorf("%s holds a conflict marker", p)
			}
			return nil
		})
	}
}

// add installs another skill from the same repo into the same project.
func (f fx) add(t *testing.T, name string, files map[string]string) fx {
	t.Helper()
	g := f
	g.name = name
	g.upstream(t, files)
	res, err := install.Add(context.Background(), f.d, install.AddRequest{
		Spec: "file://" + filepath.ToSlash(f.repo) + "//skills/" + name, Targets: []string{"claude"},
	})
	if err != nil {
		t.Fatal(err)
	}
	g.local = res.Skills[0].Dirs[0]
	return g
}

func (f fx) lock(t *testing.T) lock.Lock {
	t.Helper()
	loc, err := install.Locate(f.d, false)
	if err != nil {
		t.Fatal(err)
	}
	l, err := lock.Read(loc.LockPath)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func backupDir(t *testing.T, out string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if _, after, ok := strings.Cut(l, "backup: "); ok {
			return after
		}
	}
	t.Fatalf("no backup line in %q", out)
	return ""
}

type fakePrompter struct {
	choice string
	asked  []string
}

func (p *fakePrompter) Choose(skill, file string, c merge.Conflict, rec merge.Recommendation) (string, error) {
	p.asked = append(p.asked, skill+":"+file+":"+rec.Choice)
	return p.choice, nil
}

// conflictAndClean: code-review conflicts, clean gets a clean upstream change.
func conflictAndClean(t *testing.T) (fx, fx) {
	f := codeReview(t)
	g := f.add(t, "clean", map[string]string{"SKILL.md": skillMD("clean", "v1\n")})
	g.upstream(t, map[string]string{"SKILL.md": skillMD("clean", "v2\n")})
	return f, g
}

func TestAnyConflictStopsAllWrites(t *testing.T) {
	f, _ := conflictAndClean(t)
	before := f.snapshot(t)
	_, err := f.apply(t, Options{}, nil)
	if err == nil {
		t.Fatal("apply with an unresolved conflict succeeded")
	}
	for _, want := range []string{"nothing was written", "code-review claude SKILL.md", "RECOMMEND theirs [local-small]",
		"cali update code-review --apply --resolve SKILL.md=theirs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
	sameSnapshot(t, before, f.snapshot(t))

	// A --resolve for a file that is not a conflict is refused, not ignored.
	if _, err := f.apply(t, Options{Names: []string{"code-review"}, Resolve: map[string]string{"code-review:nope.md": "theirs"}}, nil); err == nil ||
		!strings.Contains(err.Error(), "nope.md") {
		t.Fatalf("err = %v", err)
	}
	sameSnapshot(t, before, f.snapshot(t))
}

func TestNamedSkillUpdatesWhileOtherConflicts(t *testing.T) {
	f, g := conflictAndClean(t)
	old := f.lock(t)
	out, err := f.apply(t, Options{Names: []string{"clean"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := f.lock(t)
	if l.Skills["code-review"] != old.Skills["code-review"] || l.Skills["clean"].Commit == old.Skills["clean"].Commit {
		t.Fatalf("lock %+v -> %+v", old, l)
	}
	if got := read(t, filepath.Join(g.local, "SKILL.md")); got != skillMD("clean", "v2\n") {
		t.Fatalf("clean SKILL.md = %q", got)
	}
	if got := read(t, filepath.Join(f.local, "SKILL.md")); !strings.Contains(got, "line 1 local") {
		t.Fatalf("code-review SKILL.md = %q", got)
	}
	want := "updated clean " + old.Skills["clean"].Commit[:7] + " -> " + l.Skills["clean"].Commit[:7] + "\n"
	if out != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
}

func TestTheirsWritesBackupAndLocalPatch(t *testing.T) {
	f := codeReview(t)
	oldTree := f.lock(t).Skills["code-review"].Tree
	ours, err := skillfs.ReadDir(f.local)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.apply(t, Options{Names: []string{"code-review"}, Resolve: map[string]string{"code-review:SKILL.md": "theirs"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(f.local, "SKILL.md")); got != skillMD("code-review", lines("new", 1, 12)) {
		t.Fatalf("SKILL.md = %q", got)
	}
	bk := backupDir(t, out)
	saved, err := skillfs.ReadDir(filepath.Join(bk, "files"))
	if err != nil {
		t.Fatal(err)
	}
	if !exact(saved, ours) {
		t.Fatalf("backup = %+v, want %+v", saved, ours)
	}
	// A copy of the pristine plus local.patch reproduces the old folder.
	cp := t.TempDir()
	base, ok, err := store.Get(f.d.Dirs.Data, oldTree)
	if err != nil || !ok {
		t.Fatalf("pristine %v %v", ok, err)
	}
	if err := base.WriteTo(filepath.Join(cp, "w")); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(bk, "local.patch")
	git(t, filepath.Join(cp, "w"), "apply", "--check", "-p1", patch)
	git(t, filepath.Join(cp, "w"), "apply", "-p1", patch)
	got, err := skillfs.ReadDir(filepath.Join(cp, "w"))
	if err != nil {
		t.Fatal(err)
	}
	if !exact(got, ours) {
		t.Fatalf("pristine + local.patch = %+v, want %+v", got, ours)
	}
}

func TestKeepMineAllFilesLeavesLockUntouched(t *testing.T) {
	f := codeReview(t)
	before := f.snapshot(t)
	out, err := f.apply(t, Options{KeepMine: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "kept code-review: ") {
		t.Fatalf("out = %q", out)
	}
	sameSnapshot(t, before, f.snapshot(t))
}

func TestMixedChoiceAdvancesLockAndShowsModified(t *testing.T) {
	f := codeReview(t)
	write(t, f.local, "notes.md", "my notes\n")
	old := f.lock(t)
	_, err := f.apply(t, Options{Names: []string{"code-review"},
		Resolve: map[string]string{"code-review:SKILL.md": "theirs", "code-review:notes.md": "ours"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := f.lock(t)
	if l.Skills["code-review"].Commit == old.Skills["code-review"].Commit {
		t.Fatal("lock did not advance")
	}
	if got := read(t, filepath.Join(f.local, "notes.md")); got != "my notes\n" {
		t.Fatalf("notes.md = %q", got)
	}
	loc, _ := install.Locate(f.d, false)
	m, err := manifest.ReadWith(loc.Manifest, source.Options{AllowFile: true})
	if err != nil {
		t.Fatal(err)
	}
	rows, _, err := status.Compute(context.Background(), status.Input{Scope: targets.Project, Root: loc.Root, Home: f.d.Dirs.Home,
		DataDir: f.d.Dirs.Data, Manifest: m, Lock: l, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].States) != 1 || rows[0].States[0] != status.Modified {
		t.Fatalf("rows = %+v", rows)
	}
}

// twoTargets: x installed into claude and copilot, both locally edited,
// upstream changes another file.
func twoTargets(t *testing.T) (fx, []string) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", ""), "a.md": "a\n"}, "claude", "copilot")
	dirs := []string{f.local, filepath.Join(f.d.Cwd, ".github", "skills", "x")}
	for _, d := range dirs {
		write(t, d, "SKILL.md", skillMD("x", "mine\n"))
	}
	f.upstream(t, map[string]string{"a.md": "a v2\n"})
	return f, dirs
}

func failSecondStaging(f *fx) {
	n := 0
	f.d.Rename = func(oldpath, newpath string) error {
		if strings.HasPrefix(filepath.Base(oldpath), ".cali-staging-") {
			if n++; n == 2 {
				return errors.New("injected rename failure")
			}
		}
		return os.Rename(oldpath, newpath)
	}
}

func TestStagedSwapRestoresOnRenameFailure(t *testing.T) {
	f, dirs := twoTargets(t)
	var before []skillfs.Tree
	for _, d := range dirs {
		tr, err := skillfs.ReadDir(d)
		if err != nil {
			t.Fatal(err)
		}
		before = append(before, tr)
	}
	failSecondStaging(&f)
	if _, err := f.apply(t, Options{}, nil); err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("err = %v", err)
	}
	for i, d := range dirs {
		got, err := skillfs.ReadDir(d)
		if err != nil {
			t.Fatal(err)
		}
		if !exact(got, before[i]) {
			t.Errorf("%s = %+v, want %+v", d, got, before[i])
		}
		ents, err := os.ReadDir(filepath.Dir(d))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), ".cali-") {
				t.Errorf("leftover %s", filepath.Join(filepath.Dir(d), e.Name()))
			}
		}
	}
}

func TestLockWrittenLast(t *testing.T) {
	f, _ := twoTargets(t)
	loc, _ := install.Locate(f.d, false)
	old := read(t, loc.LockPath)
	p := f.plan(t)
	failSecondStaging(&f)
	if _, err := f.apply(t, Options{}, nil); err == nil {
		t.Fatal("apply succeeded")
	}
	if got := read(t, loc.LockPath); got != old {
		t.Fatalf("lock changed:\n%s", got)
	}
	// The pristine is stored before any swap; the lock comes after them all.
	if _, ok, err := store.Get(f.d.Dirs.Data, p.Skills[0].ToTree); err != nil || !ok {
		t.Fatalf("pristine of new tree: %v %v", ok, err)
	}
	f.d.Rename = nil
	if _, err := f.apply(t, Options{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.lock(t).Skills["x"]; got.Tree != p.Skills[0].ToTree || got.Commit != p.Skills[0].ToCommit {
		t.Fatalf("lock = %+v", got)
	}
}

func TestPristineStoredForNewTree(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	f.upstream(t, map[string]string{"more.md": "more\n"})
	if _, err := f.apply(t, Options{}, nil); err != nil {
		t.Fatal(err)
	}
	e := f.lock(t).Skills["x"]
	pr, ok, err := store.Get(f.d.Dirs.Data, e.Tree)
	if err != nil || !ok {
		t.Fatalf("pristine %s: %v %v", e.Tree, ok, err)
	}
	got, err := skillfs.ReadDir(f.local)
	if err != nil {
		t.Fatal(err)
	}
	if !exact(got, pr) || len(got) != 2 {
		t.Fatalf("installed %+v, pristine %+v", got, pr)
	}
}

func TestAcceptRecommendedRefusesUnsafe(t *testing.T) {
	f := setup(t, "fm", map[string]string{"SKILL.md": skillMD("fm", "body\n"), "logo.bin": "\x00base"})
	write(t, f.local, "SKILL.md", strings.Replace(skillMD("fm", "body\n"), "The fm skill.", "Mine.", 1))
	write(t, f.local, "logo.bin", "\x00mine")
	f.upstream(t, map[string]string{
		"SKILL.md": strings.Replace(skillMD("fm", "body\n"), "The fm skill.", "Theirs.", 1),
		"logo.bin": "\x00up",
	})
	before := f.snapshot(t)
	_, err := f.apply(t, Options{AcceptRecommended: true}, nil)
	if err == nil || !strings.Contains(err.Error(), "SKILL.md [frontmatter]") || !strings.Contains(err.Error(), "logo.bin [binary]") {
		t.Fatalf("err = %v", err)
	}
	sameSnapshot(t, before, f.snapshot(t))

	// All safe: the recommendation (theirs) is applied.
	g := codeReview(t)
	if _, err := g.apply(t, Options{AcceptRecommended: true}, nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(g.local, "SKILL.md")); got != skillMD("code-review", lines("new", 1, 12)) {
		t.Fatalf("SKILL.md = %q", got)
	}
}

func TestBaseLostNeedsTakeUpstream(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	write(t, f.local, "SKILL.md", skillMD("x", "mine\n"))
	f.upstream(t, map[string]string{"more.md": "more\n"})
	if err := os.RemoveAll(filepath.Join(f.d.Dirs.Data, "pristine")); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot(t)
	if _, err := f.apply(t, Options{}, nil); err == nil || !strings.Contains(err.Error(), "x: base lost, use --take-upstream") {
		t.Fatalf("err = %v", err)
	}
	sameSnapshot(t, before, f.snapshot(t))

	out, err := f.apply(t, Options{TakeUpstream: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(f.local, "SKILL.md")); got != skillMD("x", "") {
		t.Fatalf("SKILL.md = %q", got)
	}
	bk := backupDir(t, out)
	if got := read(t, filepath.Join(bk, "files", "SKILL.md")); got != skillMD("x", "mine\n") {
		t.Fatalf("backup SKILL.md = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(bk, "local.patch")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("local.patch: %v", err)
	}
}

func TestApplyNeverWritesMarkers(t *testing.T) {
	// Adjacent edits on CRLF text: merged by ResolveDisjoint, written back as CRLF.
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", ""), "ref.md": "a\nb\nc\nd\n"})
	write(t, f.local, "ref.md", "a\r\nB\r\nc\r\nd\r\n")
	f.upstream(t, map[string]string{"ref.md": "a\nb\nC\nd\n"})
	if d := decision(t, f.plan(t), "ref.md"); d.Action != merge.Conflicted || d.Rec.Class != merge.ClassDisjointHunks {
		t.Fatalf("ref.md = %+v", d)
	}
	pr := &fakePrompter{choice: "merge"}
	if _, err := f.apply(t, Options{}, pr); err != nil {
		t.Fatal(err)
	}
	if len(pr.asked) != 1 || pr.asked[0] != "x:ref.md:merge" {
		t.Fatalf("asked %v", pr.asked)
	}
	if got := read(t, filepath.Join(f.local, "ref.md")); got != "a\r\nB\r\nC\r\nd\r\n" {
		t.Fatalf("ref.md = %q", got)
	}
}

func TestPrompterSkipKeepsSkill(t *testing.T) {
	f := codeReview(t)
	before := f.snapshot(t)
	pr := &fakePrompter{choice: "skip"}
	out, err := f.apply(t, Options{}, pr)
	if err != nil || !strings.HasPrefix(out, "kept code-review: ") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	sameSnapshot(t, before, f.snapshot(t))
}

func TestMissingTargetInstallsUpstream(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	f.upstream(t, map[string]string{"more.md": "more\n"})
	if err := os.RemoveAll(f.local); err != nil {
		t.Fatal(err)
	}
	if _, err := f.apply(t, Options{}, nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(f.local, "more.md")); got != "more\n" {
		t.Fatalf("more.md = %q", got)
	}
}

// exact compares trees byte for byte, Exec bits included.
func exact(a, b skillfs.Tree) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Path != b[i].Path || a[i].Exec != b[i].Exec || !bytes.Equal(a[i].Data, b[i].Data) {
			return false
		}
	}
	return true
}

func TestPromptKeepMineDoesNotAdvance(t *testing.T) {
	f := codeReview(t)
	before := f.snapshot(t)
	pr := &fakePrompter{choice: "ours"}
	out, err := f.apply(t, Options{}, pr)
	if err != nil || !strings.HasPrefix(out, "kept code-review: ") || len(pr.asked) != 1 {
		t.Fatalf("out = %q, err = %v, asked %v", out, err, pr.asked)
	}
	sameSnapshot(t, before, f.snapshot(t))
}

func TestEditBeforeSwapIsNotLost(t *testing.T) {
	f := codeReview(t)
	loc, _ := install.Locate(f.d, false)
	oldLock := read(t, loc.LockPath)
	f.d.Rename = func(oldpath, newpath string) error {
		if oldpath == f.local { // moving the installed folder aside: an editor saves first
			write(t, f.local, "late.md", "saved late\n")
		}
		return os.Rename(oldpath, newpath)
	}
	_, err := f.apply(t, Options{Names: []string{"code-review"}, Resolve: map[string]string{"code-review:SKILL.md": "theirs"}}, nil)
	if err == nil || !strings.Contains(err.Error(), f.local) {
		t.Fatalf("err = %v", err)
	}
	if got := read(t, filepath.Join(f.local, "late.md")); got != "saved late\n" {
		t.Fatalf("late.md = %q", got)
	}
	if got := read(t, filepath.Join(f.local, "SKILL.md")); !strings.Contains(got, "line 1 local") {
		t.Fatalf("SKILL.md = %q", got)
	}
	if got := read(t, loc.LockPath); got != oldLock {
		t.Fatalf("lock changed:\n%s", got)
	}
}

func TestLineEndingOnlyEditIsBackedUp(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", ""), "a.md": "a\nb\n"})
	write(t, f.local, "a.md", "a\r\nb\r\n")
	f.upstream(t, map[string]string{"more.md": "more\n"})
	out, err := f.apply(t, Options{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(backupDir(t, out), "files", "a.md")); got != "a\r\nb\r\n" {
		t.Fatalf("backup a.md = %q", got)
	}
}

func TestSwapRefusesSymlinkSwappedIn(t *testing.T) {
	f, dirs := twoTargets(t)
	loc, _ := install.Locate(f.d, false)
	oldLock := read(t, loc.LockPath)
	ghSkills := filepath.Dir(dirs[1])
	elsewhere := filepath.Join(t.TempDir(), "elsewhere")
	swapped := false
	f.d.Rename = func(oldpath, newpath string) error {
		if !swapped && strings.HasPrefix(filepath.Base(oldpath), ".cali-staging-") {
			// After the first target's swap: replace .github/skills with a symlink.
			swapped = true
			if err := os.Rename(ghSkills, elsewhere); err != nil {
				return err
			}
			if err := os.Symlink(elsewhere, ghSkills); err != nil {
				return err
			}
		}
		return os.Rename(oldpath, newpath)
	}
	_, err := f.apply(t, Options{}, nil)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("err = %v", err)
	}
	if got := read(t, filepath.Join(dirs[0], "SKILL.md")); got != skillMD("x", "mine\n") {
		t.Fatalf("first target not restored: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dirs[0], "a.md")); err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(dirs[0], "a.md")); got != "a\n" {
		t.Fatalf("first target a.md = %q", got)
	}
	if got := read(t, loc.LockPath); got != oldLock {
		t.Fatalf("lock changed:\n%s", got)
	}
	if got := read(t, filepath.Join(elsewhere, "x", "a.md")); got != "a\n" {
		t.Fatalf("write went through the symlink: a.md = %q", got)
	}
}

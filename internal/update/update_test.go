package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/dirs"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/merge"
)

type fx struct {
	d                 install.Deps
	name, repo, local string // local is the installed claude folder
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.invalid"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, dir, rel, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillMD(name, body string) string {
	return "---\nname: " + name + "\ndescription: The " + name + " skill.\n---\n# " + name + "\n" + body
}

// setup installs skills/<name> (files relative to the skill folder) from a
// fresh file:// repo into the claude target (or tgs) of an isolated project.
func setup(t *testing.T, name string, files map[string]string, tgs ...string) fx {
	t.Helper()
	base := t.TempDir()
	empty := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f := fx{name: name, repo: filepath.Join(base, "repo")}
	project := filepath.Join(base, "project")
	for _, d := range []string{f.repo, project, filepath.Join(base, "home")} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	git(t, f.repo, "init", "-q", "-b", "main")
	for p, c := range files {
		write(t, f.repo, "skills/"+name+"/"+p, c)
	}
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "one")
	f.d = install.Deps{
		Dirs: dirs.Dirs{
			Config: filepath.Join(base, "config"), Data: filepath.Join(base, "data"),
			Cache: filepath.Join(base, "cache"), Home: filepath.Join(base, "home"),
		},
		Getenv:          func(string) string { return "" },
		GhToken:         func() (string, error) { return "", errors.New("no gh") },
		AllowFileScheme: true,
		Cwd:             project,
	}
	if tgs == nil {
		tgs = []string{"claude"}
	}
	res, err := install.Add(context.Background(), f.d, install.AddRequest{
		Spec: "file://" + filepath.ToSlash(f.repo) + "//skills/" + name, Targets: tgs,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.local = res.Skills[0].Dirs[0]
	return f
}

// upstream writes files into the skill folder of the repo and commits.
func (f fx) upstream(t *testing.T, files map[string]string) {
	t.Helper()
	for p, c := range files {
		write(t, f.repo, "skills/"+f.name+"/"+p, c)
	}
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "upstream")
}

func (f fx) plan(t *testing.T) Proposal {
	t.Helper()
	p, err := Plan(context.Background(), f.d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// snapshot hashes every file and dir (with its mode) under the scope's
// home, config, data and project dirs; the cache mirror may change.
func (f fx) snapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range []string{f.d.Dirs.Home, f.d.Dirs.Config, f.d.Dirs.Data, f.d.Cwd} {
		err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			if err != nil {
				return err
			}
			fi, err := e.Info()
			if err != nil {
				return err
			}
			h := fi.Mode().String()
			if fi.Mode().IsRegular() {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				s := sha256.Sum256(b)
				h += " " + hex.EncodeToString(s[:])
			}
			out[p] = h
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func sameSnapshot(t *testing.T, before, after map[string]string) {
	t.Helper()
	for p, h := range before {
		if after[p] != h {
			t.Errorf("%s changed: %q -> %q", p, h, after[p])
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok {
			t.Errorf("%s was created", p)
		}
	}
}

func lines(prefix string, from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		fmt.Fprintf(&b, "%s %d\n", prefix, i)
	}
	return b.String()
}

func decision(t *testing.T, p Proposal, path string) FileDecision {
	t.Helper()
	if len(p.Skills) != 1 || len(p.Skills[0].Targets) != 1 {
		t.Fatalf("proposal = %+v", p)
	}
	for _, d := range p.Skills[0].Targets[0].Files {
		if d.Path == path {
			return d
		}
	}
	t.Fatalf("no decision for %s in %+v", path, p.Skills[0].Targets[0].Files)
	return FileDecision{}
}

// codeReview: local edits one body line, upstream rewrites the body, so the
// conflict is local-small and Cali recommends theirs.
func codeReview(t *testing.T) fx {
	f := setup(t, "code-review", map[string]string{
		"SKILL.md": skillMD("code-review", lines("line", 1, 12)),
		"notes.md": "notes\n",
	})
	write(t, f.local, "SKILL.md", skillMD("code-review", "line 1 local\n"+lines("line", 2, 12)))
	f.upstream(t, map[string]string{
		"SKILL.md": skillMD("code-review", lines("new", 1, 12)),
		"notes.md": "notes v2\n",
	})
	return f
}

func TestDryRunWritesNothing(t *testing.T) {
	f := codeReview(t)
	write(t, f.repo, "skills/code-review/added.md", "added\n")
	f.upstream(t, nil)
	before := f.snapshot(t)
	p := f.plan(t)
	var out bytes.Buffer
	if err := p.WriteText(&out, false); err != nil {
		t.Fatal(err)
	}
	if err := p.WriteJSON(&out); err != nil {
		t.Fatal(err)
	}
	sameSnapshot(t, before, f.snapshot(t))
	if d := decision(t, p, "added.md"); d.Action != merge.TakeUpstream {
		t.Fatalf("added.md = %+v", d)
	}
}

func TestNoSubtreeChangeIsNoop(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	write(t, f.repo, "README.md", "elsewhere\n")
	git(t, f.repo, "add", ".")
	git(t, f.repo, "commit", "-q", "-m", "readme")
	p := f.plan(t)
	if len(p.Skills) != 1 || !p.Skills[0].Noop || p.Skills[0].FromTree != p.Skills[0].ToTree || p.Skills[0].Targets != nil {
		t.Fatalf("proposal = %+v", p)
	}
	var out bytes.Buffer
	if err := p.WriteText(&out, false); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "x: up to date\n0 skill(s) to update, 0 conflict(s)\n"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
	if _, err := Plan(context.Background(), f.d, Options{Names: []string{"nope"}}); err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("unknown name: err = %v", err)
	}
}

func TestNonTTYConflictPrintsCommandAndChangesNothing(t *testing.T) {
	f := codeReview(t)
	before := f.snapshot(t)
	p := f.plan(t)
	d := decision(t, p, "SKILL.md")
	if d.Action != merge.Conflicted || d.Rec == nil || d.Rec.Class != merge.ClassLocalSmall || d.Rec.Choice != "theirs" ||
		d.LocalLines != 2 || d.UpstreamLines != 24 {
		t.Fatalf("SKILL.md = %+v rec=%+v", d, d.Rec)
	}
	var out bytes.Buffer
	if err := p.WriteText(&out, false); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"code-review " + p.Skills[0].FromCommit[:7] + " -> " + p.Skills[0].ToCommit[:7] + "\n",
		"  claude notes.md: take-upstream\n",
		"  claude SKILL.md: CONFLICT\n",
		"@@ ours line 6\n",
		"ours| line 1 local\n",
		"base| line 1\n",
		"theirs| new 1\n",
		"RECOMMEND theirs [local-small] (" + d.Rec.Reason + ")\n",
		"cali update code-review --apply --resolve SKILL.md=theirs\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output lacks %q:\n%s", want, s)
		}
	}
	if !strings.HasSuffix(s, "\n1 skill(s) to update, 1 conflict(s)\n") {
		t.Errorf("summary:\n%s", s)
	}
	if p.Conflicts() != 1 {
		t.Errorf("Conflicts() = %d", p.Conflicts())
	}
	sameSnapshot(t, before, f.snapshot(t))
}

func TestAcceptRecommendedRefusesOnBothLargeBinaryFrontmatter(t *testing.T) {
	f := setup(t, "fm", map[string]string{
		"SKILL.md": skillMD("fm", "body\n"),
		"big.md":   lines("base", 1, 30),
		"logo.bin": "\x00\x01base",
	})
	write(t, f.local, "SKILL.md", strings.Replace(skillMD("fm", "body\n"), "The fm skill.", "Mine.", 1))
	write(t, f.local, "big.md", lines("mine", 1, 15)+lines("base", 16, 30))
	write(t, f.local, "logo.bin", "\x00\x01mine")
	f.upstream(t, map[string]string{
		"SKILL.md": strings.Replace(skillMD("fm", "body\n"), "The fm skill.", "Theirs.", 1),
		"big.md":   lines("up", 1, 15) + lines("base", 16, 30),
		"logo.bin": "\x00\x01up",
	})
	p := f.plan(t)
	for path, class := range map[string]merge.Class{
		"SKILL.md": merge.ClassFrontmatter, "big.md": merge.ClassBothLarge, "logo.bin": merge.ClassBinary,
	} {
		d := decision(t, p, path)
		if d.Action != merge.Conflicted || d.Rec == nil || d.Rec.Class != class || d.Rec.Safe || d.Rec.Choice != "ours" {
			t.Errorf("%s = %+v rec=%+v, want unsafe %s", path, d, d.Rec, class)
		}
	}
}

func TestPlanBaseLost(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	f.upstream(t, map[string]string{"more.md": "more\n"})
	if err := os.RemoveAll(filepath.Join(f.d.Dirs.Data, "pristine")); err != nil {
		t.Fatal(err)
	}
	p := f.plan(t)
	if len(p.Skills) != 1 || !p.Skills[0].BaseLost || p.Skills[0].Noop || p.Skills[0].Targets != nil {
		t.Fatalf("proposal = %+v", p)
	}
	var out bytes.Buffer
	if err := p.WriteText(&out, false); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "x: base lost: run cali sync to rebuild it; if sync reports base-lost, use --take-upstream\n") {
		t.Fatalf("out = %q", out.String())
	}
}

func TestPlanMissingTarget(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	f.upstream(t, map[string]string{"more.md": "more\n"})
	if err := os.RemoveAll(f.local); err != nil {
		t.Fatal(err)
	}
	p := f.plan(t)
	if len(p.Skills) != 1 || len(p.Skills[0].Targets) != 1 {
		t.Fatalf("proposal = %+v", p)
	}
	if tp := p.Skills[0].Targets[0]; !tp.Missing || tp.Files != nil || tp.Target != "claude" || tp.Dir != f.local {
		t.Fatalf("target = %+v", tp)
	}
}

func TestPlanPreResolvesWhitespace(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", ""), "notes.md": "a\nb\nc\n"})
	write(t, f.local, "notes.md", "a\nb  \r\nc\n")
	f.upstream(t, map[string]string{"notes.md": "a\nB\nc\n"})
	d := decision(t, f.plan(t), "notes.md")
	if d.Action != merge.TakeUpstream || !d.PreResolved || d.Rec != nil || d.Conflict != nil || string(d.Merged) != "a\nB\nc\n" {
		t.Fatalf("notes.md = %+v", d)
	}
}

func TestPlanPreResolvesWhitespaceKeepsLocalDeletion(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", ""), "notes.md": "a\nb\nc\n"})
	if err := os.Remove(filepath.Join(f.local, "notes.md")); err != nil {
		t.Fatal(err)
	}
	f.upstream(t, map[string]string{"notes.md": "a\nb  \nc\n"})
	d := decision(t, f.plan(t), "notes.md")
	if d.Action != merge.KeepLocal || !d.Delete || !d.PreResolved || d.Rec != nil || d.Conflict != nil {
		t.Fatalf("notes.md = %+v", d)
	}
}

func TestDryRunJSONShape(t *testing.T) {
	f := codeReview(t)
	var out bytes.Buffer
	if err := f.plan(t).WriteJSON(&out); err != nil {
		t.Fatal(err)
	}
	var got struct {
		SchemaVersion int `json:"schema_version"`
		Skills        []struct {
			Name, From, To string
			Noop           *bool `json:"noop"`
			BaseLost       *bool `json:"base_lost"`
			Targets        []struct {
				Target, Dir string
				Missing     *bool `json:"missing"`
				Files       []map[string]json.RawMessage
			}
		}
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if got.SchemaVersion != 1 || len(got.Skills) != 1 || got.Skills[0].Name != "code-review" || len(got.Skills[0].From) != 40 ||
		len(got.Skills[0].To) != 40 || got.Skills[0].Noop == nil || got.Skills[0].BaseLost == nil || len(got.Skills[0].Targets) != 1 {
		t.Fatalf("got %+v\n%s", got, out.String())
	}
	tg := got.Skills[0].Targets[0]
	if tg.Target != "claude" || tg.Dir != f.local || tg.Missing == nil || len(tg.Files) != 2 {
		t.Fatalf("target %+v", tg)
	}
	files := map[string]map[string]json.RawMessage{}
	for _, fl := range tg.Files {
		var p string
		_ = json.Unmarshal(fl["path"], &p)
		files[p] = fl
	}
	if n := files["notes.md"]; string(n["action"]) != `"take-upstream"` || string(n["delete"]) != "false" || n["conflict"] != nil {
		t.Fatalf("notes.md = %s", out.String())
	}
	var c struct {
		Class, Choice, Reason string
		Safe                  bool
		LocalLines            int `json:"local_lines"`
		UpstreamLines         int `json:"upstream_lines"`
		Hunks                 []struct {
			OursStart          int `json:"ours_start"`
			Ours, Base, Theirs []string
		}
	}
	s := files["SKILL.md"]
	if string(s["action"]) != `"conflict"` || json.Unmarshal(s["conflict"], &c) != nil {
		t.Fatalf("SKILL.md = %s", out.String())
	}
	if c.Class != "local-small" || c.Choice != "theirs" || !c.Safe || c.Reason == "" || c.LocalLines != 2 || c.UpstreamLines != 24 ||
		len(c.Hunks) != 1 || c.Hunks[0].OursStart != 6 || c.Hunks[0].Ours[0] != "line 1 local" || c.Hunks[0].Theirs[0] != "new 1" {
		t.Fatalf("conflict = %+v", c)
	}
}

func TestShellArgQuotesRepoPaths(t *testing.T) {
	for in, want := range map[string]string{
		"SKILL.md=theirs":       "SKILL.md=theirs",
		"a b.md=ours":           "'a b.md=ours'",
		"$(touch x)'.md=theirs": `'$(touch x)'\''.md=theirs'`,
		"scripts/run.sh=merge":  "scripts/run.sh=merge",
	} {
		if got := shellArg(in); got != want {
			t.Errorf("shellArg(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanWarnsAboutLeftovers(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	left := filepath.Join(filepath.Dir(f.local), ".cali-old-123")
	if err := os.Mkdir(left, 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	f.d.Stdout = &out
	if _, err := Plan(context.Background(), f.d, Options{}); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "warning: leftover from an interrupted update: "+left+"\n"; got != want {
		t.Fatalf("out = %q, want %q", got, want)
	}
	if _, err := os.Lstat(left); err != nil {
		t.Fatalf("leftover removed: %v", err)
	}
}

func TestPlanRefusesLockWithoutManifest(t *testing.T) {
	f := setup(t, "x", map[string]string{"SKILL.md": skillMD("x", "")})
	manifestPath := filepath.Join(f.d.Cwd, "cali.yaml")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if _, err := Plan(context.Background(), f.d, Options{}); err == nil || !strings.Contains(err.Error(), manifestPath) {
		t.Fatalf("err = %v, want an error naming %s", err, manifestPath)
	}
}

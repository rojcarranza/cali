package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/source"
)

// Integration with a real agent-skills clone (github.com/rojcarranza/agent-skills),
// the main consumer of cali.yaml. Skipped unless CALI_AGENT_SKILLS names that
// clone. Needs network and a token that can read it (GITHUB_TOKEN or gh auth):
//
//	CALI_AGENT_SKILLS=~/Dev/projects/agent-skills go test -v -run AgentSkills ./internal/cli
func agentSkills(t *testing.T) (repo, manifestPath string) {
	t.Helper()
	clone := os.Getenv("CALI_AGENT_SKILLS")
	if clone == "" {
		t.Skip("CALI_AGENT_SKILLS is not set")
	}
	clone, err := filepath.Abs(clone) // before t.Chdir below
	if err != nil {
		t.Fatal(err)
	}
	// Resolve the gh token while HOME is still real: gh finds no login in the scratch HOME.
	if os.Getenv("GITHUB_TOKEN") == "" && os.Getenv("GH_TOKEN") == "" {
		if tok, err := ghToken(); err == nil {
			t.Setenv("GITHUB_TOKEN", tok)
		}
	}
	base := t.TempDir()
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("USERPROFILE", filepath.Join(base, "home"))
	t.Setenv("CALI_CONFIG_DIR", filepath.Join(base, "config"))
	t.Setenv("CALI_DATA_DIR", filepath.Join(base, "data"))
	t.Setenv("CALI_CACHE_DIR", filepath.Join(base, "cache"))
	t.Chdir(base)
	// A clone, so a run never rewrites the clone's cali.lock, and own skills
	// (.//skills/<name>) are read at its committed HEAD.
	repo = filepath.Join(base, "m")
	gitRun(t, base, "clone", "-q", "--no-hardlinks", clone, repo)
	return repo, filepath.Join(repo, "cali.yaml")
}

// installedAsDeclared checks every manifest skill is in each of its targets and nowhere else.
func installedAsDeclared(t *testing.T, manifestPath string, o source.Options) {
	t.Helper()
	m, err := manifest.ReadWith(manifestPath, o)
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("HOME")
	roots := map[string]string{"claude": filepath.Join(home, ".claude", "skills"), "copilot": filepath.Join(home, ".copilot", "skills")}
	for name := range m.Skills {
		want := map[string]bool{}
		for _, tg := range m.TargetsFor(name) {
			want[tg] = true
		}
		for tg, root := range roots {
			_, err := os.Stat(filepath.Join(root, name, "SKILL.md"))
			if want[tg] && err != nil {
				t.Errorf("%s not installed for %s: %v", name, tg, err)
			}
			if !want[tg] && err == nil {
				t.Errorf("%s installed for %s, which its targets leave out", name, tg)
			}
		}
	}
}

// What install.py runs: the committed lock installs as is, then stays put.
func TestAgentSkillsFrozenSync(t *testing.T) {
	_, mp := agentSkills(t)
	before, err := os.ReadFile(filepath.Join(filepath.Dir(mp), "cali.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if code, out, errs := run("sync", "-g", "--manifest", mp, "--frozen"); code != 0 {
		t.Fatalf("sync --frozen: code=%d out=%q err=%q", code, out, errs)
	}
	installedAsDeclared(t, mp, source.Options{})
	if after, _ := os.ReadFile(filepath.Join(filepath.Dir(mp), "cali.lock")); !bytes.Equal(before, after) {
		t.Error("sync --frozen changed cali.lock")
	}
	if code, out, errs := run("sync", "-g", "--manifest", mp, "--frozen"); code != 0 || out != "" {
		t.Errorf("second sync is not a no-op: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("status", "-g", "--manifest", mp, "--offline", "--exit-code"); code != 0 {
		t.Errorf("status --offline after sync: code=%d out=%q err=%q", code, out, errs)
	}
}

// The committed lock matches HEAD and upstream: an own skill committed
// without a lock bump fails --frozen, and status reports nothing outdated.
func TestAgentSkillsLockIsCurrent(t *testing.T) {
	_, mp := agentSkills(t)
	if code, out, errs := run("sync", "-g", "--manifest", mp, "--frozen"); code != 0 {
		t.Fatalf("sync --frozen: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("status", "-g", "--manifest", mp, "--exit-code"); code != 0 {
		t.Errorf("cali.lock is behind upstream: code=%d err=%q\n%s", code, errs, out)
	}
}

// Adding a skill to agent-skills in one PR: the skill and its cali.yaml entry
// without a lock entry fail --frozen (what CI runs), and a plain sync locks
// and installs it from the clone before anything is merged.
func TestAgentSkillsNewSkill(t *testing.T) {
	repo, mp := agentSkills(t)
	skill := filepath.Join(repo, "skills", "roj-new-skill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: roj-new-skill\ndescription: A new skill. Use when testing.\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(mp)
	if err != nil {
		t.Fatal(err)
	}
	s := strings.Replace(string(b), "\nskills:\n", "\nskills:\n  roj-new-skill:\n    source: .//skills/roj-new-skill\n", 1)
	if err := os.WriteFile(mp, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".")
	gitRun(t, repo, "commit", "-q", "-m", "add roj-new-skill")

	if code, out, errs := run("sync", "-g", "--manifest", mp, "--frozen"); code != 1 || !strings.Contains(out+errs, "roj-new-skill") {
		t.Fatalf("sync --frozen with an unlocked skill: code=%d out=%q err=%q", code, out, errs)
	}
	if code, out, errs := run("sync", "-g", "--manifest", mp); code != 0 {
		t.Fatalf("sync: code=%d out=%q err=%q", code, out, errs)
	}
	installedAsDeclared(t, mp, source.Options{})
	l, err := lock.Read(filepath.Join(repo, "cali.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if e := l.Skills["roj-new-skill"]; e.Source != "." || e.Path != "skills/roj-new-skill" || e.Commit != "" || len(e.Tree) != 40 {
		t.Errorf("lock entry = %+v", e)
	}
	if code, out, errs := run("sync", "-g", "--manifest", mp, "--frozen"); code != 0 {
		t.Errorf("sync --frozen once locked: code=%d out=%q err=%q", code, out, errs)
	}
}

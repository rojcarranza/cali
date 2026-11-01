package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Intent: SCR-14 piece 3, cali doctor flags duplicates, leftovers, skipped
// names, Claude-only skills and unmet declared needs.

func skill(t *testing.T, base, rel, name, front string) {
	t.Helper()
	dir := filepath.Join(base, filepath.FromSlash(rel), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "---\n" + front + "---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

func std(name string) string { return "name: " + name + "\ndescription: D.\n" }

func kinds(r Report) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Kind+":"+f.Skill)
	}
	return out
}

func has(r Report, want string) bool {
	for _, k := range kinds(r) {
		if k == want {
			return true
		}
	}
	return false
}

func TestDoctorFlagsEachProblem(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	// installed for both default targets: VS Code reads both folders.
	skill(t, root, ".github/skills", "both", std("both"))
	skill(t, root, ".claude/skills", "both", std("both"))
	// dropped from cali.yaml, still loaded at home.
	skill(t, home, ".copilot/skills", "gone", std("gone"))
	skill(t, home, ".copilot/skills", "kept", std("kept"))
	// VS Code skips a folder whose SKILL.md name differs.
	skill(t, home, ".copilot/skills", "Mismatch", std("other"))
	skill(t, home, ".copilot/skills", "nodesc", "name: nodesc\n")
	// Claude-only key and declared needs.
	skill(t, home, ".copilot/skills", "cc", std("cc")+"allowed-tools: Bash\nmetadata:\n  cali-env: ATLASSIAN_TOKEN, HOME_SET\n  cali-tools: jira, sh\n")

	in := Input{Root: root, Home: home,
		LookPath: func(n string) (string, error) {
			if n == "sh" {
				return "/bin/sh", nil
			}
			return "", errors.New("no")
		},
		Getenv: func(k string) string {
			if k == "HOME_SET" {
				return "x"
			}
			return ""
		}}
	in.Managed.Global = map[string]bool{"kept": true, "cc": true}
	r := Run(in)

	for _, want := range []string{"duplicate:both", "leftover:gone", "skipped:Mismatch", "skipped:nodesc", "claude-only:cc", "needs:cc"} {
		if !has(r, want) {
			t.Errorf("missing %s in %v", want, kinds(r))
		}
	}
	for _, bad := range []string{"leftover:kept", "leftover:both", "duplicate:kept", "duplicate:gone"} {
		if has(r, bad) {
			t.Errorf("unexpected %s in %v", bad, kinds(r))
		}
	}
	var needs []string
	for _, f := range r.Findings {
		if f.Kind == "needs" {
			needs = append(needs, f.Detail)
		}
	}
	if got := strings.Join(needs, "|"); got != "environment variable ATLASSIAN_TOKEN is not set|tool jira is not on PATH" {
		t.Errorf("needs = %q", got)
	}
}

func TestDoctorNoManifestMeansNoLeftovers(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	skill(t, home, ".copilot/skills", "x", std("x"))
	r := Run(Input{Root: root, Home: home})
	if len(r.Findings) != 0 {
		t.Fatalf("findings = %v", kinds(r))
	}
}

func TestDoctorListsFoldersPerApp(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	skill(t, home, ".claude/skills", "c", std("c"))
	r := Run(Input{Root: root, Home: home})
	if len(r.Apps) != 3 || r.Apps[2].Name != "Claude Code" || len(r.Apps[2].Folders[1].Skills) != 1 {
		t.Fatalf("apps = %+v", r.Apps)
	}
	// JetBrains reads only ~/.copilot/skills.
	if f := r.Apps[1].Folders; len(f) != 1 || f[0].Dir != filepath.Join(home, ".copilot", "skills") {
		t.Fatalf("jetbrains folders = %+v", f)
	}
}

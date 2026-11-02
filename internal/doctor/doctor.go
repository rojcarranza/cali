// Package doctor inspects the skill folders the Copilot apps and Claude Code
// read, and reports what is wrong with them (ADR 0006). It reads only.
package doctor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
)

// Input is what Run inspects. Managed holds the skill names each scope's
// cali.yaml lists; a nil map means that scope has no cali.yaml, and nothing in
// it is called a leftover. An empty Root means there is no project. LookPath and Getenv are os/exec.LookPath and os.Getenv.
type Input struct {
	Root, Home string
	Managed    struct{ Project, Global map[string]bool }
	LookPath   func(string) (string, error)
	Getenv     func(string) string
}

// Folder is one skills folder an app reads.
type Folder struct {
	Dir    string
	Scope  string // "project" or "personal"
	Skills []string
	// managed is true for the two folders cali installs into.
	managed bool
	set     map[string]bool // names the matching cali.yaml lists, nil if none
}

// App is a client and the folders it reads.
type App struct {
	Name    string
	Folders []*Folder
}

// Finding is one problem. Kind is duplicate, leftover, skipped, claude-only or needs.
type Finding struct{ Kind, Skill, Detail string }

// Report is the result of Run.
type Report struct {
	Apps     []App
	Findings []Finding
}

// claudeOnly are SKILL.md keys Claude Code acts on and the Copilot apps ignore.
var claudeOnly = []string{"allowed-tools", "model", "effort", "context", "agent", "hooks", "shell", "paths"}

// Run scans the folders and builds the report. Folders and findings are sorted.
func Run(in Input) Report {
	pf := func(rel string, managed bool) *Folder {
		if in.Root == "" { // no project: an empty folder, so nothing is read
			return &Folder{Dir: "(no project)", Scope: "project"}
		}
		return &Folder{Dir: filepath.Join(in.Root, filepath.FromSlash(rel)), Scope: "project", managed: managed, set: in.Managed.Project}
	}
	hf := func(rel string, managed bool) *Folder {
		return &Folder{Dir: filepath.Join(in.Home, filepath.FromSlash(rel)), Scope: "personal", managed: managed, set: in.Managed.Global}
	}
	ghp, clp, agp := pf(".github/skills", true), pf(".claude/skills", true), pf(".agents/skills", false)
	cop, clh, agh := hf(".copilot/skills", true), hf(".claude/skills", true), hf(".agents/skills", false)
	r := Report{Apps: []App{
		{"VS Code Copilot Chat", []*Folder{ghp, clp, agp, cop, clh, agh}},
		{"JetBrains Copilot", []*Folder{cop}}, // ADR 0006 V1: per the agent-skills guide, not checked on a client
		{"Claude Code", []*Folder{clp, clh}},
	}}

	seen := map[*Folder]bool{}
	var all []*Folder
	for _, a := range r.Apps {
		for _, f := range a.Folders {
			if !seen[f] {
				seen[f] = true
				all = append(all, f)
			}
		}
	}

	notes := map[string]bool{} // findings once per skill, whatever folder it is in
	add := func(kind, skill, detail string) {
		r.Findings = append(r.Findings, Finding{kind, skill, detail})
	}
	for _, f := range all {
		ents, _ := os.ReadDir(f.Dir)
		for _, e := range ents {
			name := e.Name()
			if !e.IsDir() || strings.HasPrefix(name, ".") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(f.Dir, name, "SKILL.md"))
			if err != nil {
				continue // not a skill folder
			}
			f.Skills = append(f.Skills, name)
			if f.managed && f.set != nil && !f.set[name] {
				add("leftover", name, f.Dir+": not in cali.yaml; delete it, or add it")
			}
			fm, err := skillfs.ParseFrontmatter(data)
			switch {
			case err != nil:
				add("skipped", name, f.Dir+": "+err.Error()+"; VS Code skips it")
				continue
			case fm.Name != name:
				add("skipped", name, f.Dir+": SKILL.md name does not match the folder name; VS Code skips it")
				continue
			case !manifest.SkillNameValid(name):
				add("skipped", name, f.Dir+": name must be lowercase letters, digits and hyphens; VS Code skips it")
				continue
			case fm.Description == "":
				add("skipped", name, f.Dir+": SKILL.md has no description; VS Code skips it")
				continue
			}
			if notes[name] {
				continue
			}
			notes[name] = true
			var keys []string
			for _, k := range fm.Keys {
				for _, c := range claudeOnly {
					if k == c {
						keys = append(keys, k)
					}
				}
			}
			if len(keys) > 0 {
				add("claude-only", name, "uses "+strings.Join(keys, ", ")+", which Copilot ignores")
			}
			for _, v := range split(fm.Metadata["cali-env"]) {
				if in.Getenv == nil || in.Getenv(v) == "" {
					add("needs", name, "environment variable "+v+" is not set")
				}
			}
			for _, t := range split(fm.Metadata["cali-tools"]) {
				if in.LookPath == nil {
					continue
				}
				alts := strings.Split(t, "|") // python3|python: any one on PATH passes
				ok := false
				for i, a := range alts {
					alts[i] = strings.TrimSpace(a)
					if onPath(in.LookPath, alts[i]) {
						ok = true
					}
				}
				if !ok {
					if len(alts) == 1 {
						add("needs", name, "tool "+t+" is not on PATH")
					} else {
						add("needs", name, "none of the tools "+strings.Join(alts, ", ")+" is on PATH")
					}
				}
			}
		}
	}

	for _, a := range r.Apps {
		count := map[string][]string{}
		for _, f := range a.Folders {
			for _, s := range f.Skills {
				count[s] = append(count[s], f.Dir)
			}
		}
		for s, dirs := range count {
			if len(dirs) > 1 {
				add("duplicate", s, a.Name+" reads it in "+strings.Join(dirs, " and ")+"; it may list twice (ADR 0006 V2)")
			}
		}
	}
	for _, f := range all {
		sort.Strings(f.Skills)
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Skill != b.Skill {
			return a.Skill < b.Skill
		}
		return a.Detail < b.Detail
	})
	return r
}

func split(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// onPath reports whether program t is on PATH. Windows ships python and python3
// stubs in the WindowsApps folder that open the Store; they are not Python.
func onPath(lookPath func(string) (string, error), t string) bool {
	p, err := lookPath(t)
	if err != nil {
		return false
	}
	low := strings.ToLower(strings.ReplaceAll(p, "/", "\\"))
	if strings.Contains(low, "\\windowsapps\\") {
		base := strings.TrimSuffix(strings.ToLower(t), ".exe")
		return base != "python" && base != "python3"
	}
	return true
}

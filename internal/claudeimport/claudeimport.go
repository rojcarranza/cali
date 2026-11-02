// Package claudeimport lists the skills of the Claude Code plugins installed
// on this computer as cali.yaml entries, each pinned to the commit Claude
// installed. It only reads Claude's files under ~/.claude.
package claudeimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/source"
)

// Candidate is one skill that an installed, switched-on plugin provides.
type Candidate struct {
	Name   string // install name, the skill folder name
	Plugin string // "plugin@marketplace"
	Source string // cali source spec, github:owner/repo//path/to/skill
	Ref    string // the commit Claude installed
}

// Entry is the cali.yaml entry for c.
func (c Candidate) Entry() manifest.Skill { return manifest.Skill{Source: c.Source, Ref: c.Ref} }

// Result is what Scan found. Notes explain every plugin or skill left out.
type Result struct {
	Candidates []Candidate
	Notes      []string
}

type installed struct {
	Plugins map[string][]struct {
		Scope        string `json:"scope"`
		InstallPath  string `json:"installPath"`
		GitCommitSha string `json:"gitCommitSha"`
	} `json:"plugins"`
}

type marketplaceSource struct {
	Source string `json:"source"`
	Repo   string `json:"repo"`
	URL    string `json:"url"`
}

// pluginSource is a marketplace plugin's "source": a path inside the
// marketplace repository, or a repository of its own.
type pluginSource struct {
	local string
	marketplaceSource
	Path string
}

func (p *pluginSource) UnmarshalJSON(b []byte) error {
	if json.Unmarshal(b, &p.local) == nil {
		return nil
	}
	if err := json.Unmarshal(b, &p.marketplaceSource); err != nil {
		return err
	}
	var sub struct {
		Path string `json:"path"`
	}
	err := json.Unmarshal(b, &sub)
	p.Path = sub.Path
	return err
}

// Scan reads claudeDir (normally ~/.claude). A plugin Claude has switched off
// in settings.json, or installed only for one project, is left out.
func Scan(claudeDir string) (Result, error) {
	var inst installed
	if err := readJSON(filepath.Join(claudeDir, "plugins", "installed_plugins.json"), &inst); err != nil {
		return Result{}, fmt.Errorf("no Claude plugins found: %w", err)
	}
	var settings struct {
		Enabled map[string]bool `json:"enabledPlugins"`
	}
	_ = readJSON(filepath.Join(claudeDir, "settings.json"), &settings) // no settings means all on
	var markets map[string]struct {
		Source          marketplaceSource `json:"source"`
		InstallLocation string            `json:"installLocation"`
	}
	_ = readJSON(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), &markets)

	var res Result
	note := func(f string, a ...any) { res.Notes = append(res.Notes, fmt.Sprintf(f, a...)) }
	keys := make([]string, 0, len(inst.Plugins))
	for k := range inst.Plugins {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	taken := map[string]string{}
	for _, key := range keys {
		pluginName, market, _ := strings.Cut(key, "@")
		if on, ok := settings.Enabled[key]; ok && !on {
			note("%s: switched off in Claude, skipped", key)
			continue
		}
		var in struct{ path, sha string }
		for _, e := range inst.Plugins[key] {
			if e.Scope == "user" {
				in.path, in.sha = e.InstallPath, e.GitCommitSha
			}
		}
		if in.path == "" {
			note("%s: installed for a project only, skipped", key)
			continue
		}
		if in.sha == "" {
			note("%s: Claude recorded no commit, skipped", key)
			continue
		}
		repo, dir, err := locate(markets[market].InstallLocation, markets[market].Source, pluginName)
		if err != nil {
			note("%s: %v, skipped", key, err)
			continue
		}
		for _, root := range skillRoots(in.path) {
			ents, _ := os.ReadDir(filepath.Join(in.path, filepath.FromSlash(root)))
			for _, e := range ents {
				if !e.IsDir() {
					continue
				}
				if _, err := os.Stat(filepath.Join(in.path, filepath.FromSlash(root), e.Name(), "SKILL.md")); err != nil {
					continue
				}
				if !manifest.SkillNameValid(e.Name()) {
					note("%s: skill folder %q is not a valid cali name, skipped", key, e.Name())
					continue
				}
				if other, dup := taken[e.Name()]; dup {
					note("%s: skill %s is already provided by %s, skipped", key, e.Name(), other)
					continue
				}
				taken[e.Name()] = key
				res.Candidates = append(res.Candidates, Candidate{
					Name: e.Name(), Plugin: key, Ref: in.sha,
					Source: repo + "//" + path.Join(dir, root, e.Name()),
				})
			}
		}
	}
	return res, nil
}

// locate gives the cali repo spec (github:owner/repo or an https URL) and
// the plugin's folder inside that repository, from the marketplace listing.
func locate(marketDir string, ms marketplaceSource, plugin string) (repo, dir string, err error) {
	var mf struct {
		Plugins []struct {
			Name   string       `json:"name"`
			Source pluginSource `json:"source"`
		} `json:"plugins"`
	}
	if marketDir == "" {
		return "", "", errors.New("marketplace not known")
	}
	if err := readJSON(filepath.Join(marketDir, ".claude-plugin", "marketplace.json"), &mf); err != nil {
		return "", "", fmt.Errorf("cannot read its marketplace: %w", err)
	}
	for _, p := range mf.Plugins {
		if p.Name != plugin {
			continue
		}
		if p.Source.local != "" { // a folder of the marketplace repository
			r, err := repoSpec(ms)
			return r, strings.Trim(path.Clean(p.Source.local), "./"), err
		}
		r, err := repoSpec(p.Source.marketplaceSource)
		return r, strings.Trim(path.Clean(p.Source.Path), "./"), err
	}
	return "", "", errors.New("not listed in its marketplace")
}

func repoSpec(s marketplaceSource) (string, error) {
	switch {
	case s.Source == "github" && s.Repo != "":
		return "github:" + s.Repo, nil
	case strings.HasPrefix(s.URL, "https://github.com/"):
		return "github:" + strings.TrimSuffix(strings.TrimPrefix(s.URL, "https://github.com/"), ".git"), nil
	case strings.HasPrefix(s.URL, "https://"):
		return s.URL, nil
	}
	return "", errors.New("its source is not a github or https repository")
}

// skillRoots are the plugin-relative folders holding skill folders: the
// skills entry of plugin.json, else skills.
func skillRoots(installPath string) []string {
	var pj struct {
		Skills json.RawMessage `json:"skills"`
	}
	_ = readJSON(filepath.Join(installPath, ".claude-plugin", "plugin.json"), &pj)
	var list []string
	var one string
	if json.Unmarshal(pj.Skills, &one) == nil && one != "" {
		list = []string{one}
	} else {
		_ = json.Unmarshal(pj.Skills, &list)
	}
	var roots []string
	for _, s := range list {
		s = path.Clean(strings.TrimPrefix(s, "./"))
		if s != "." && s != ".." && !strings.HasPrefix(s, "../") && !strings.HasPrefix(s, "/") {
			roots = append(roots, s)
		}
	}
	if len(roots) == 0 {
		roots = []string{"skills"}
	}
	return roots
}

func readJSON(p string, v any) error {
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Valid checks every candidate source parses, so a bad spec never reaches cali.yaml.
func Valid(c Candidate) error {
	_, err := source.Parse(c.Source)
	return err
}

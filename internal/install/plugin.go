package install

import (
	"encoding/json"
	"path"
	"slices"
	"strings"

	"github.com/rojcarranza/cali/internal/gitx"
)

// pluginDir is the folder a Claude plugin or marketplace repository uses for
// its manifests.
const pluginDir = ".claude-plugin"

// stringList is a JSON value that is one string or a list of strings.
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*s = stringList{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		*s = nil // an unusable value is ignored, not an error
		return nil
	}
	*s = many
	return nil
}

// pluginManifest is the part of .claude-plugin/plugin.json and
// .claude-plugin/marketplace.json that names skill folders.
type pluginManifest struct {
	Skills  stringList `json:"skills"`
	Plugins []struct {
		Source json.RawMessage `json:"source"`
		Skills stringList      `json:"skills"`
	} `json:"plugins"`
}

// pluginSkillRoots returns the repo folders a Claude plugin repository says
// hold its skills, read from .claude-plugin/plugin.json (the repository is
// one plugin) or .claude-plugin/marketplace.json (it lists plugins at
// relative paths). A plugin without a skills entry uses its skills/ folder.
// ok is false when the repository is neither, so the caller scans it all.
func pluginSkillRoots(repo *gitx.Repo, commit string) (roots []string, ok bool) {
	read := func(name string, v *pluginManifest) bool {
		data, err := repo.ReadFile(commit, pluginDir+"/"+name)
		return err == nil && json.Unmarshal(data, v) == nil
	}
	var m pluginManifest
	switch {
	case read("plugin.json", &m):
		roots = skillRoots("", m.Skills)
	case read("marketplace.json", &m):
		for _, p := range m.Plugins {
			var src string
			if json.Unmarshal(p.Source, &src) != nil {
				continue // a plugin from another repository is not in this one
			}
			dir := cleanRel(src)
			if dir == "" && src != "" && src != "./" && src != "." {
				continue
			}
			roots = append(roots, skillRoots(dir, p.Skills)...)
		}
	default:
		return nil, false
	}
	return roots, true
}

// skillRoots resolves a plugin's skills entries against its folder dir; no
// entry means dir/skills.
func skillRoots(dir string, skills []string) []string {
	if len(skills) == 0 {
		return []string{path.Join(dir, "skills")}
	}
	var out []string
	for _, s := range skills {
		if r := cleanRel(s); r != "" {
			out = append(out, path.Join(dir, r))
		}
	}
	return out
}

// cleanRel cleans a plugin-relative path ("./skills"); a path that is empty
// or leaves the repository gives "".
func cleanRel(p string) string {
	p = path.Clean(strings.TrimPrefix(p, "./"))
	if p == "." || p == ".." || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/") {
		return ""
	}
	return p
}

// onlyPluginSkills keeps the skill folders in found that sit at or under a
// root. It returns found unchanged when nothing matches, so a plugin that
// names no real folder still installs what the scan finds.
func onlyPluginSkills(found, roots []string) []string {
	kept := slices.DeleteFunc(slices.Clone(found), func(p string) bool {
		return !slices.ContainsFunc(roots, func(r string) bool { return p == r || strings.HasPrefix(p, r+"/") })
	})
	if len(kept) == 0 {
		return found
	}
	return kept
}

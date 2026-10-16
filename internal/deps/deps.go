// Package deps works out which skills a skill requires and which are missing.
package deps

import (
	"slices"
	"sort"
	"strings"

	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/skillfs"
)

// Warning says skill Skill, installed for Target, lacks the required skill Missing.
type Warning struct{ Skill, Missing, Target string }

// Requires returns the sorted, deduplicated union of the manifest requires and
// the comma-separated metadata["cali-requires"] of the SKILL.md.
func Requires(m manifest.Skill, fm skillfs.Frontmatter) []string {
	all := slices.Clone(m.Requires)
	for _, v := range strings.Split(fm.Metadata["cali-requires"], ",") {
		if v = strings.TrimSpace(v); v != "" {
			all = append(all, v)
		}
	}
	sort.Strings(all)
	return slices.Compact(all)
}

// Check reports, per target, each required skill that is not installed there.
// installed maps target to the skill names present (project and global merged
// by the caller); requires maps skill to its dependencies. A skill is only
// checked for targets it is installed in.
func Check(installed map[string][]string, requires map[string][]string) []Warning {
	var out []Warning
	for _, skill := range sortedKeys(requires) {
		for _, target := range sortedKeys(installed) {
			have := installed[target]
			if !slices.Contains(have, skill) {
				continue
			}
			for _, dep := range requires[skill] {
				if !slices.Contains(have, dep) {
					out = append(out, Warning{Skill: skill, Missing: dep, Target: target})
				}
			}
		}
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

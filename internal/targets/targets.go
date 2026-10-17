// Package targets maps an agent target and scope to its skills directory.
package targets

import (
	"fmt"
	"path/filepath"
)

// Scope is where a skill is installed: in a project or in the user's home.
type Scope int

// Install scopes.
const (
	Project Scope = iota
	Global
)

// table holds the skills directory, relative to the project root or home,
// for each target and scope.
var table = map[string]map[Scope]string{
	"claude":  {Project: ".claude/skills", Global: ".claude/skills"},
	"copilot": {Project: ".github/skills", Global: ".copilot/skills"},
}

// Dir returns the skills directory (not the per-skill folder) for target in
// scope. Project scope is rooted at projectRoot, Global at home.
func Dir(target string, scope Scope, projectRoot, home string) (string, error) {
	scopes, ok := table[target]
	if !ok {
		return "", fmt.Errorf("unknown target %q, want claude or copilot", target)
	}
	rel, ok := scopes[scope]
	if !ok {
		return "", fmt.Errorf("unknown scope %d", int(scope))
	}
	base := projectRoot
	if scope == Global {
		base = home
	}
	return filepath.Join(base, filepath.FromSlash(rel)), nil
}

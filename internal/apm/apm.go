// Package apm reads an APM manifest (apm.yml) and maps its skills to cali
// manifest entries. Only skills are imported: MCP servers and includes are
// reported in Result.Skipped.
package apm

import (
	"fmt"
	"io"
	"path"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/source"
)

// Result is what Parse found. Skills is keyed by install name. Skipped holds
// one warning line per ignored entry. Targets is the file's top-level targets.
type Result struct {
	Skills  map[string]manifest.Skill
	Skipped []string
	Targets []string
}

type file struct {
	Includes     yaml.Node `yaml:"includes"`
	Targets      []string  `yaml:"targets"`
	Dependencies struct {
		APM []yaml.Node `yaml:"apm"`
		MCP []yaml.Node `yaml:"mcp"`
	} `yaml:"dependencies"`
}

type object struct {
	Git     string   `yaml:"git"`
	Path    string   `yaml:"path"`
	Ref     string   `yaml:"ref"`
	Targets []string `yaml:"targets"`
}

// Parse reads apm.yml from r. localSource is the source spec of the repo that
// holds the local "./x/y" entries (e.g. github:owner/repo); when empty those
// entries are skipped with a warning.
func Parse(r io.Reader, localSource string) (Result, error) {
	var f file
	if err := yaml.NewDecoder(r).Decode(&f); err != nil {
		return Result{}, fmt.Errorf("parse apm.yml: %w", err)
	}
	res := Result{Skills: map[string]manifest.Skill{}, Targets: f.Targets}
	if !f.Includes.IsZero() {
		res.Skipped = append(res.Skipped, "includes is ignored")
	}
	for i, n := range f.Dependencies.APM {
		var s manifest.Skill
		switch {
		case n.Kind == yaml.ScalarNode && strings.HasPrefix(n.Value, "./"):
			if localSource == "" {
				res.Skipped = append(res.Skipped, "local skill "+n.Value+" needs --local-source")
				continue
			}
			// A ref on localSource must move to Ref: "src@ref//path" does not parse.
			src, ref := localSource, ""
			if i := strings.LastIndex(src, "@"); i >= 0 {
				src, ref = src[:i], src[i+1:]
			}
			s.Source, s.Ref = src+"//"+strings.Trim(n.Value[2:], "/"), ref
		case n.Kind == yaml.ScalarNode:
			spec, ref, _ := strings.Cut(n.Value, "#")
			p := strings.SplitN(spec, "/", 3)
			if len(p) < 3 || p[2] == "" {
				return Result{}, fmt.Errorf("apm entry %d: want owner/repo/path[#ref]", i)
			}
			s = manifest.Skill{Source: "github:" + p[0] + "/" + p[1] + "//" + p[2], Ref: ref}
		case n.Kind == yaml.MappingNode:
			var o object
			if err := n.Decode(&o); err != nil || o.Git == "" || o.Path == "" {
				return Result{}, fmt.Errorf("apm entry %d: want git and path", i)
			}
			s = manifest.Skill{
				Source:  strings.TrimSuffix(strings.TrimRight(o.Git, "/"), ".git") + "//" + strings.Trim(o.Path, "/"),
				Ref:     o.Ref,
				Targets: o.Targets,
			}
		default:
			return Result{}, fmt.Errorf("apm entry %d: unsupported shape", i)
		}
		sp, err := source.Parse(s.Source)
		if err != nil {
			return Result{}, fmt.Errorf("apm entry %d: %w", i, err)
		}
		name := path.Base(sp.Path)
		if !manifest.SkillNameValid(name) {
			return Result{}, fmt.Errorf("apm entry %d: install name %q is not valid", i, name)
		}
		if _, dup := res.Skills[name]; dup {
			return Result{}, fmt.Errorf("duplicate install name %s (apm entry %d)", name, i)
		}
		res.Skills[name] = s
	}
	for _, n := range f.Dependencies.MCP {
		id := n.Value
		if n.Kind == yaml.MappingNode {
			var o struct {
				Name string `yaml:"name"`
			}
			_ = n.Decode(&o)
			id = o.Name
		}
		res.Skipped = append(res.Skipped, "mcp server "+id+" is not managed by cali")
	}
	return res, nil
}

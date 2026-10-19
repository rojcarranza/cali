package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	skilldeps "github.com/rojcarranza/cali/internal/deps"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/targets"
)

func newShowCmd(out io.Writer, global *bool) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show one skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			d, err := deps(out)
			if err != nil {
				return err
			}
			scopes := []bool{*global}
			if !*global && manifestPath == "" { // not in the project: try the global scope
				scopes = append(scopes, true)
			}
			states, err := loadScopes(d, scopes, io.Discard)
			if err != nil {
				return err
			}
			var s state
			var sk manifest.Skill
			ok := false
			for _, st := range states {
				if sk, ok = st.m.Skills[name]; ok {
					s = st
					break
				}
			}
			if !ok {
				return fmt.Errorf("no skill named %s in the manifest", name)
			}
			type tgt struct {
				Target    string `json:"target"`
				Dir       string `json:"dir"`
				Installed bool   `json:"installed"`
			}
			tgts := []tgt{}
			var fm skillfs.Frontmatter
			haveFM := false
			for _, t := range s.m.TargetsFor(name) {
				base, err := targets.Dir(t, s.loc.Scope, s.loc.Root, s.home)
				if err != nil {
					return err
				}
				dir := filepath.Join(base, name)
				fi, err := os.Stat(dir)
				tgts = append(tgts, tgt{t, dir, err == nil && fi.IsDir()})
				if !haveFM && err == nil {
					if data, err := os.ReadFile(filepath.Join(dir, "SKILL.md")); err == nil {
						if f, err := skillfs.ParseFrontmatter(data); err == nil {
							fm, haveFM = f, true
						}
					}
				}
			}
			e := s.l.Skills[name]
			req := skilldeps.Requires(sk, fm)
			if asJSON {
				return render.WriteJSON(out, map[string]any{
					"name": name, "source": sk.Source, "ref": sk.Ref, "path": e.Path,
					"commit": e.Commit, "tree": e.Tree, "targets": tgts, "requires": req,
				})
			}
			// Values come from a possibly cloned manifest and a repo's SKILL.md: strip control characters.
			w := func(k, v string) { _, _ = io.WriteString(out, k+": "+render.Sanitize(v)+"\n") }
			w("name", name)
			w("source", sk.Source)
			w("ref", sk.Ref)
			w("path", dash(e.Path))
			w("commit", dash(e.Commit))
			w("tree", dash(e.Tree))
			for _, t := range tgts {
				state := "missing"
				if t.Installed {
					state = "installed"
				}
				w("target "+t.Target, t.Dir+" ("+state+")")
			}
			w("requires", strings.Join(req, ","))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

package cli

import (
	"errors"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/status"
)

// errFindings makes `status --exit-code` exit 1 without an error message.
var errFindings = errors.New("status found problems")

func newStatusCmd(out io.Writer, global *bool) *cobra.Command {
	var asJSON, offline, exitCode bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local edits, newer upstream versions and missing skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := load(out, *global)
			if err != nil {
				return err
			}
			d, err := deps(out)
			if err != nil {
				return err
			}
			in := status.Input{
				Scope: s.loc.Scope, Root: s.loc.Root, Home: s.home, DataDir: d.Dirs.Data,
				Manifest: s.m, Lock: s.l, Offline: offline,
				Fetcher:       &status.GitFetcher{Ctx: cmd.Context(), CacheDir: d.Dirs.Cache, Getenv: d.Getenv, GhToken: d.GhToken},
				SourceOptions: source.Options{AllowFile: allowFileScheme},
				Warn:          cmd.ErrOrStderr(),
			}
			if !*global {
				g, err := load(out, true)
				if err != nil {
					return err
				}
				in.Global = g.m
			}
			rows, warns, err := status.Compute(cmd.Context(), in)
			if err != nil {
				return err
			}

			type warning struct {
				Skill   string `json:"skill"`
				Missing string `json:"missing"`
				Target  string `json:"target"`
			}
			jw := []warning{}
			for _, w := range warns {
				jw = append(jw, warning{w.Skill, w.Missing, w.Target})
			}
			mode := render.ModeFor(isTTY(out), asJSON)
			if mode == render.JSON {
				err = render.WriteJSON(out, struct {
					Rows     []status.Row `json:"rows"`
					Warnings []warning    `json:"warnings"`
				}{rows, jw})
			} else {
				t := render.Table{Header: []string{"SKILL", "TARGET", "STATE", "DETAIL"}}
				for _, r := range rows {
					st := make([]string, len(r.States))
					for i, x := range r.States {
						st[i] = string(x)
					}
					t.Rows = append(t.Rows, []string{r.Skill, r.Target, strings.Join(st, ","), r.Detail})
				}
				t.Render(out, mode)
				seen := map[string]bool{}
				for _, w := range warns {
					// One line per skill and dependency, even when several targets lack it.
					line := "WARN " + w.Skill + " requires " + w.Missing + " (missing)\n"
					if !seen[line] {
						seen[line] = true
						_, err = io.WriteString(out, line)
					}
				}
			}
			if err != nil {
				return err
			}
			if exitCode {
				if len(warns) > 0 {
					return errFindings
				}
				for _, r := range rows {
					for _, x := range r.States {
						if x != status.OK && x != status.Shadowed {
							return errFindings
						}
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&offline, "offline", false, "do not fetch; skip the check for newer upstream versions")
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit 1 when any skill is not ok or a dependency is missing")
	return cmd
}

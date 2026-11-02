package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/doctor"
	"github.com/rojcarranza/cali/internal/render"
)

func newDoctorCmd(out io.Writer, global *bool) *cobra.Command {
	var exitCode, asJSON bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Show the skill folders each app reads and flag problems",
		Long: "Show the skill folders VS Code, JetBrains and Claude Code read, and flag duplicates,\n" +
			"leftovers (not in cali.yaml), names VS Code skips, Claude-only skills and unmet declared needs\n" +
			"(metadata cali-env and cali-tools). It looks at the project and your home folder; with -g only at your home\n" +
			"folder, which --windows-home points at the Windows home from WSL.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := deps(out)
			if err != nil {
				return err
			}
			in := doctor.Input{Home: d.Dirs.Home, LookPath: exec.LookPath, Getenv: os.Getenv}
			if *global { // -g: personal folders only, in d.Dirs.Home (the Windows home with --windows-home)
			} else if p, err := loadFrom(d, false); err != nil {
				// e.g. run from the home folder: only the personal folders are checked
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: project scope skipped: %s\n", err)
			} else {
				in.Root = p.loc.Root
				if _, err := os.Stat(p.loc.Manifest); err == nil {
					in.Managed.Project = names(p)
				}
			}
			gd := d
			gd.Manifest = "" // --manifest names the project manifest
			g, err := loadFrom(gd, true)
			if err != nil {
				return err
			}
			if _, err := os.Stat(g.loc.Manifest); err == nil {
				in.Managed.Global = names(g)
			}

			r := doctor.Run(in)
			if render.ModeFor(isTTY(out), asJSON) == render.JSON {
				type folder struct {
					Dir    string   `json:"dir"`
					Scope  string   `json:"scope"`
					Skills []string `json:"skills"`
				}
				type app struct {
					Name    string   `json:"name"`
					Folders []folder `json:"folders"`
				}
				type finding struct {
					Kind   string `json:"kind"`
					Skill  string `json:"skill"`
					Detail string `json:"detail"`
				}
				j := struct {
					Apps     []app     `json:"apps"`
					Findings []finding `json:"findings"`
				}{[]app{}, []finding{}}
				for _, a := range r.Apps {
					x := app{Name: a.Name, Folders: []folder{}}
					for _, f := range a.Folders {
						sk := f.Skills
						if sk == nil {
							sk = []string{}
						}
						x.Folders = append(x.Folders, folder{f.Dir, f.Scope, sk})
					}
					j.Apps = append(j.Apps, x)
				}
				for _, f := range r.Findings {
					j.Findings = append(j.Findings, finding{f.Kind, f.Skill, f.Detail})
				}
				if err := render.WriteJSON(out, j); err != nil {
					return err
				}
			} else {
				total := fmt.Sprintf("%d problems\n", len(r.Findings))
				if agentMode { // totals first (AXI rule 5)
					_, _ = io.WriteString(out, total)
				}
				for _, a := range r.Apps {
					_, _ = fmt.Fprintln(out, a.Name)
					for _, f := range a.Folders {
						_, _ = fmt.Fprintf(out, "  %s (%s): %d skills\n", f.Dir, f.Scope, len(f.Skills))
					}
				}
				if len(r.Findings) > 0 {
					_, _ = fmt.Fprintln(out)
				}
				for _, f := range r.Findings {
					_, _ = fmt.Fprintf(out, "%-11s %s: %s\n", f.Kind, render.Sanitize(f.Skill), render.Sanitize(f.Detail))
				}
				if !agentMode {
					_, _ = fmt.Fprint(out, "\n"+total)
				}
			}
			if len(r.Findings) == 0 {
				return nil
			}
			if exitCode {
				return errFindings
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&exitCode, "exit-code", false, "exit 1 when doctor finds a problem")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

func names(s state) map[string]bool {
	m := map[string]bool{}
	for n := range s.m.Skills {
		m[n] = true
	}
	return m
}

package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/merge"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/update"
)

// huhPrompter asks on the terminal, one conflicting file at a time, with the
// recommendation preselected (design §5 "How the choice is made").
type huhPrompter struct{}

func (huhPrompter) Choose(skill, file string, c merge.Conflict, rec merge.Recommendation) (string, error) {
	var b strings.Builder
	for _, h := range c.Hunks {
		fmt.Fprintf(&b, "@@ ours line %d\n", h.OursStart)
		for _, side := range []struct {
			tag   string
			lines []string
		}{{"ours", h.Ours}, {"base", h.Base}, {"theirs", h.Theirs}} {
			for _, l := range side.lines {
				fmt.Fprintf(&b, "%s| %s\n", side.tag, render.Sanitize(l))
			}
		}
	}
	fmt.Fprintf(&b, "RECOMMEND %s [%s] (%s)", rec.Choice, rec.Class, rec.Reason)
	opts := []huh.Option[string]{huh.NewOption("take-upstream", "theirs"), huh.NewOption("keep-mine", "ours")}
	if _, ok := merge.ResolveDisjoint(c); ok {
		opts = append(opts, huh.NewOption("merge both", "merge"))
	}
	opts = append(opts, huh.NewOption("skip skill", "skip"))
	choice := rec.Choice
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("%s %s: conflict", skill, render.Sanitize(file))).
		Description(b.String()).
		Options(opts...).
		Value(&choice).
		Run()
	return choice, err
}

func newUpdateCmd(out io.Writer, global *bool) *cobra.Command {
	var o update.Options
	var resolve []string
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "update [names...]",
		Short: "Show what updating skills from upstream would change (dry run); --apply writes it",
		RunE: func(cmd *cobra.Command, args []string) error {
			o.Names = args
			if len(resolve) > 0 {
				if len(args) != 1 {
					return errors.New("--resolve needs exactly one skill name")
				}
				o.Resolve = map[string]string{}
				for _, r := range resolve {
					i := strings.LastIndex(r, "=")
					if i <= 0 || (r[i+1:] != "ours" && r[i+1:] != "theirs" && r[i+1:] != "merge") {
						return fmt.Errorf("--resolve %q: want <file>=ours|theirs|merge", r)
					}
					o.Resolve[args[0]+":"+r[:i]] = r[i+1:]
				}
			}
			d, err := deps(out)
			if err != nil {
				return err
			}
			d.Global = *global
			d.Stdout = cmd.ErrOrStderr() // notes stay off stdout so --json output parses
			p, err := update.Plan(cmd.Context(), d, o)
			if err != nil {
				return err
			}
			if o.Apply {
				var pr update.Prompter
				if isTTY(out) && !asJSON {
					pr = huhPrompter{}
				}
				d.Stdout = out
				return update.Apply(cmd.Context(), d, p, pr)
			}
			if asJSON {
				err = p.WriteJSON(out)
			} else {
				err = p.WriteText(out, *global)
			}
			if err == nil && p.Conflicts() > 0 {
				return errFindings
			}
			return err
		},
	}
	f := cmd.Flags()
	f.BoolVar(&o.Apply, "apply", false, "write the update")
	f.StringArrayVar(&resolve, "resolve", nil, "resolve one conflicting file: <file>=ours|theirs|merge (repeatable, one skill)")
	f.BoolVar(&o.TakeUpstream, "take-upstream", false, "take upstream for every conflicting file")
	f.BoolVar(&o.KeepMine, "keep-mine", false, "keep the local file for every conflicting file")
	f.BoolVar(&o.AcceptRecommended, "accept-recommended", false, "apply the recommendation for every conflict")
	f.BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

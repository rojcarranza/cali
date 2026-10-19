package cli

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/merge"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/source"
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
	var skill string
	cmd := &cobra.Command{
		Use:   "update [names or sources...]",
		Short: "Show what updating skills from upstream would change (dry run); --apply writes it",
		RunE: func(cmd *cobra.Command, args []string) error {
			args, err := expandSources(out, *global, args, skill)
			if err != nil {
				return err
			}
			o.Names = args
			if o.TakeUpstream && o.KeepMine {
				return errors.New("--take-upstream and --keep-mine cannot be used together")
			}
			if o.Apply && asJSON {
				return errors.New("--apply and --json cannot be used together")
			}
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
				if isTTY(out) {
					pr = huhPrompter{}
				}
				d.Stdout = out
				return update.Apply(cmd.Context(), d, p, pr)
			}
			if asJSON {
				err = p.WriteJSON(out)
			} else {
				err = p.WriteText(out)
			}
			if err == nil && (p.Conflicts() > 0 || p.Skipped()) {
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
	f.StringVar(&skill, "skill", "", "update only the skill with this name")
	return cmd
}

// expandSources replaces each source spec in args (github:o/r, a URL, .//path)
// with the manifest skills installed from it, then keeps only skill when set.
func expandSources(out io.Writer, global bool, args []string, skill string) ([]string, error) {
	var names []string
	var s *state
	for _, a := range args {
		if a != source.Here && !strings.ContainsAny(a, ":/") {
			names = append(names, a) // a skill name
			continue
		}
		want, err := source.ParseWith(a, source.Options{AllowFile: allowFileScheme})
		if err != nil {
			return nil, err
		}
		if s == nil {
			st, err := load(out, global)
			if err != nil {
				return nil, err
			}
			s = &st
		}
		var hits []string
		for n, sk := range s.m.Skills {
			got, err := source.ParseWith(sk.Source, source.Options{AllowFile: allowFileScheme})
			if err == nil && got.URL == want.URL && (want.Path == "" || got.Path == want.Path) {
				hits = append(hits, n)
			}
		}
		if len(hits) == 0 {
			return nil, fmt.Errorf("no installed skill comes from %s", a)
		}
		sort.Strings(hits)
		names = append(names, hits...)
	}
	if skill == "" {
		return names, nil
	}
	if len(names) > 0 && !slices.Contains(names, skill) {
		return nil, fmt.Errorf("--skill %s: not installed from %s", skill, strings.Join(args, ", "))
	}
	return []string{skill}, nil
}

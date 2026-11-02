package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/claudeimport"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/manifest"
)

func newImportCmd(out io.Writer, global *bool) *cobra.Command {
	cmd := &cobra.Command{Use: "import", Short: "Import skills from another tool into cali.yaml"}
	var claudeDir string
	var skills []string
	var all bool
	c := &cobra.Command{
		Use:   "claude",
		Short: "Write skills of your installed Claude plugins into cali.yaml, pinned to Claude's commit",
		Long: "Reads ~/.claude (installed and switched-on plugins), lets you pick skills and writes them to\n" +
			"cali.yaml pinned to the commit Claude installed. Nothing is installed; run cali sync after.\n" +
			"Without --skill or --all, it asks on a terminal and otherwise lists the skills and stops.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all && len(skills) > 0 {
				return errors.New("--skill and --all cannot be used together")
			}
			if claudeDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				claudeDir = filepath.Join(home, ".claude")
			}
			res, err := claudeimport.Scan(claudeDir)
			if err != nil {
				return err
			}
			for _, n := range res.Notes {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "note: "+n)
			}
			if len(res.Candidates) == 0 {
				_, _ = fmt.Fprintln(out, "0 skills found in your switched-on Claude plugins")
				return nil
			}
			var picked []claudeimport.Candidate
			switch {
			case all:
				picked = res.Candidates
			case len(skills) > 0:
				for _, n := range skills {
					i := indexOf(res.Candidates, n)
					if i < 0 {
						return fmt.Errorf("no skill %q in your Claude plugins; run cali import claude to list them", n)
					}
					picked = append(picked, res.Candidates[i])
				}
			case interactive(out):
				picked, err = pick(out, os.Stdin, res.Candidates)
				if err != nil {
					return err
				}
			default:
				for _, c := range res.Candidates {
					_, _ = fmt.Fprintf(out, "%s\t%s\n", c.Name, c.Plugin)
				}
				_, _ = fmt.Fprintf(out, "%d skills found; pass --skill <name> (repeatable) or --all to import\n", len(res.Candidates))
				return nil
			}
			entries := map[string]manifest.Skill{}
			for _, c := range picked {
				if err := claudeimport.Valid(c); err != nil {
					return fmt.Errorf("%s: %w", c.Name, err)
				}
				entries[c.Name] = c.Entry()
			}
			d, err := deps(out)
			if err != nil {
				return err
			}
			mf, added, kept, err := install.Record(d, *global, entries)
			if err != nil {
				return err
			}
			for _, n := range kept {
				_, _ = fmt.Fprintf(out, "kept %s (already in %s)\n", n, mf)
			}
			for _, n := range added {
				_, _ = fmt.Fprintf(out, "added %s (%s) to %s\n", n, shortSha(entries[n].Ref), mf)
			}
			_, _ = fmt.Fprintf(out, "%d added, %d kept; run cali sync to install\n", len(added), len(kept))
			return nil
		},
	}
	c.Flags().StringVar(&claudeDir, "claude-dir", "", "Claude's folder (default ~/.claude)")
	c.Flags().StringSliceVar(&skills, "skill", nil, "import this skill (repeatable)")
	c.Flags().BoolVar(&all, "all", false, "import every skill found")
	cmd.AddCommand(c)
	return cmd
}

func shortSha(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func indexOf(cs []claudeimport.Candidate, name string) int {
	for i, c := range cs {
		if c.Name == name {
			return i
		}
	}
	return -1
}

// pick lists the candidates and reads numbers or names, "all" or empty (none).
func pick(out io.Writer, in io.Reader, cs []claudeimport.Candidate) ([]claudeimport.Candidate, error) {
	for i, c := range cs {
		_, _ = fmt.Fprintf(out, "%3d  %s  (%s)\n", i+1, c.Name, c.Plugin)
	}
	_, _ = fmt.Fprint(out, "Import which? numbers or names separated by spaces, all, or Enter for none: ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return nil, nil
	}
	var picked []claudeimport.Candidate
	for _, f := range strings.Fields(line) {
		if f == "all" {
			return cs, nil
		}
		if n, err := strconv.Atoi(f); err == nil && n >= 1 && n <= len(cs) {
			picked = append(picked, cs[n-1])
		} else if i := indexOf(cs, f); i >= 0 {
			picked = append(picked, cs[i])
		} else {
			return nil, fmt.Errorf("%q is not a number or skill name from the list", f)
		}
	}
	return picked, nil
}

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/apm"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/source"
)

func newImportCmd(out io.Writer, global *bool) *cobra.Command {
	var localSource string
	cmd := &cobra.Command{
		Use:   "import <apm.yml>",
		Short: "Add the skills of an apm.yml to cali.yaml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			res, err := apm.Parse(f, localSource)
			if err != nil {
				return err
			}
			d, err := deps(out)
			if err != nil {
				return err
			}
			loc, err := install.Locate(d, *global)
			if err != nil {
				return err
			}
			opts := source.Options{AllowFile: allowFileScheme}
			m, err := manifest.ReadWith(loc.Manifest, opts)
			if errors.Is(err, os.ErrNotExist) {
				ts := res.Targets
				if len(ts) == 0 {
					ts = []string{"claude", "copilot"}
				}
				m, err = manifest.Manifest{Version: 1, Targets: ts, Skills: map[string]manifest.Skill{}}, nil
			}
			if err != nil {
				return err
			}
			errw := cmd.ErrOrStderr()
			for _, w := range res.Skipped {
				_, _ = fmt.Fprintln(errw, "warning: "+w)
			}
			names := make([]string, 0, len(res.Skills))
			for n := range res.Skills {
				names = append(names, n)
			}
			sort.Strings(names)
			added := 0
			for _, n := range names {
				s := res.Skills[n]
				switch old, ok := m.Skills[n]; {
				case !ok:
					m.Skills[n] = s
					added++
				case old.Source != s.Source || old.Ref != s.Ref:
					_, _ = fmt.Fprintf(errw, "warning: skipped %s: already in cali.yaml from %s\n", n, old.Source)
				}
			}
			if err := os.MkdirAll(filepath.Dir(loc.Manifest), 0o755); err != nil {
				return err
			}
			if err := manifest.WriteWith(loc.Manifest, m, opts); err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "imported %d skill(s); run cali sync to install\n", added)
			return err
		},
	}
	cmd.Flags().StringVar(&localSource, "local-source", "", "source spec of the repo that holds the local ./ skills, e.g. github:owner/repo")
	return cmd
}

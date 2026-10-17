package cli

import (
	"errors"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/lock"
	"github.com/rojcarranza/cali/internal/manifest"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/source"
	"github.com/rojcarranza/cali/internal/status"
)

// isTTY reports whether w is a terminal; anything but an *os.File is not.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// state is what list and show read: manifest and lock of one scope. Offline.
type state struct {
	loc  install.Location
	home string
	m    manifest.Manifest
	l    lock.Lock
}

func load(out io.Writer, global bool) (state, error) {
	d, err := deps(out)
	if err != nil {
		return state{}, err
	}
	return loadFrom(d, global)
}

func loadFrom(d install.Deps, global bool) (state, error) {
	loc, err := install.Locate(d, global)
	if err != nil {
		return state{}, err
	}
	m, err := manifest.ReadWith(loc.Manifest, source.Options{AllowFile: allowFileScheme})
	if errors.Is(err, os.ErrNotExist) {
		m, err = manifest.Manifest{Version: 1, Skills: map[string]manifest.Skill{}}, nil
	}
	if err != nil {
		return state{}, err
	}
	l, err := lock.Read(loc.LockPath)
	if err != nil {
		return state{}, err
	}
	return state{loc, d.Dirs.Home, m, l}, nil
}

func short(c string) string {
	if c == "" {
		return "-"
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

func newListCmd(out io.Writer, global *bool) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			s, err := load(out, *global)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(s.m.Skills))
			for n := range s.m.Skills {
				names = append(names, n)
			}
			sort.Strings(names)
			type row struct {
				Name    string   `json:"name"`
				Source  string   `json:"source"`
				Commit  string   `json:"commit"`
				Targets []string `json:"targets"`
			}
			rows := []row{}
			t := render.Table{Header: []string{"NAME", "SOURCE", "COMMIT", "TARGETS"}}
			for _, n := range names {
				r := row{n, s.m.Skills[n].Source, s.l.Skills[n].Commit, s.m.TargetsFor(n)}
				if r.Targets == nil {
					r.Targets = []string{}
				}
				rows = append(rows, r)
				t.Rows = append(t.Rows, []string{r.Name, r.Source, short(r.Commit), strings.Join(r.Targets, ",")})
			}
			um, err := status.UnmanagedRows(s.m, s.loc.Scope, s.loc.Root, s.home)
			if err != nil {
				return err
			}
			for _, u := range um {
				rows = append(rows, row{u.Skill, "unmanaged", "", []string{u.Target}})
				t.Rows = append(t.Rows, []string{u.Skill, "unmanaged", "-", u.Target})
			}
			mode := render.ModeFor(isTTY(out), asJSON)
			if mode == render.JSON {
				return render.WriteJSON(out, struct {
					Skills []row `json:"skills"`
				}{rows})
			}
			t.Render(out, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return cmd
}

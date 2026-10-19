package cli

import (
	"errors"
	"fmt"
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
	"github.com/rojcarranza/cali/internal/targets"
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

// loadScopes loads each scope (false = project, true = global). With more than
// one scope, a scope that fails to load is a warning on warn, unless all fail.
func loadScopes(d install.Deps, scopes []bool, warn io.Writer) ([]state, error) {
	var out []state
	var first error
	for _, g := range scopes {
		s, err := loadFrom(d, g)
		if err == nil {
			out = append(out, s)
			continue
		}
		if first == nil {
			first = err
		}
		if len(scopes) > 1 {
			_, _ = fmt.Fprintf(warn, "warning: %s scope: %s\n", scopeName(g), err)
		}
	}
	if len(out) == 0 {
		return nil, first
	}
	return out, nil
}

func isGlobal(s state) bool { return s.loc.Scope == targets.Global }

func short(c string) string {
	if c == "" {
		return "-"
	}
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

type listRow struct {
	Scope   string   `json:"scope,omitempty"`
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Commit  string   `json:"commit"`
	Targets []string `json:"targets"`
}

// listRows returns the manifest skills of s, then the unmanaged folders.
func listRows(s state) ([]listRow, error) {
	names := make([]string, 0, len(s.m.Skills))
	for n := range s.m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	rows := []listRow{}
	for _, n := range names {
		r := listRow{Name: n, Source: s.m.Skills[n].Source, Commit: s.l.Skills[n].Commit, Targets: s.m.TargetsFor(n)}
		if r.Targets == nil {
			r.Targets = []string{}
		}
		rows = append(rows, r)
	}
	um, err := status.UnmanagedRows(s.m, s.loc.Scope, s.loc.Root, s.home)
	if err != nil {
		return nil, err
	}
	for _, u := range um {
		rows = append(rows, listRow{Name: u.Skill, Source: "unmanaged", Targets: []string{u.Target}})
	}
	return rows, nil
}

func scopeName(global bool) string {
	if global {
		return "global"
	}
	return "project"
}

func newListCmd(out io.Writer, global *bool) *cobra.Command {
	var asJSON, all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if all && manifestPath != "" {
				return errors.New("-a lists both scopes and cannot be used with --manifest")
			}
			scopes := []bool{*global}
			if all {
				scopes = []bool{false, true}
			}
			d, err := deps(out)
			if err != nil {
				return err
			}
			if (*global || all) && manifestPath == "" && d.Dirs.Note != "" {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s\n", d.Dirs.Note)
			}
			states, err := loadScopes(d, scopes, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			rows := []listRow{}
			for _, s := range states {
				rs, err := listRows(s)
				if err != nil {
					return err
				}
				if all {
					for i := range rs {
						rs[i].Scope = scopeName(isGlobal(s))
					}
				}
				rows = append(rows, rs...)
			}
			mode := render.ModeFor(isTTY(out), asJSON)
			if mode == render.JSON {
				return render.WriteJSON(out, struct {
					Skills []listRow `json:"skills"`
				}{rows})
			}
			t := render.Table{Header: []string{"NAME", "SOURCE", "COMMIT", "TARGETS"}}
			if all {
				t.Header = append([]string{"SCOPE"}, t.Header...)
			}
			for _, r := range rows {
				cells := []string{r.Name, r.Source, short(r.Commit), strings.Join(r.Targets, ",")}
				if all {
					cells = append([]string{r.Scope}, cells...)
				}
				t.Rows = append(t.Rows, cells)
			}
			t.Render(out, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "list project and global skills")
	return cmd
}

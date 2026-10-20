package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/gitx"
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

// yesNo is a table cell: "-" for unknown.
func yesNo(b *bool) string {
	switch {
	case b == nil:
		return "-"
	case *b:
		return "yes"
	}
	return "no"
}

type listRow struct {
	Scope   string   `json:"scope,omitempty"`
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Managed bool     `json:"managed"`
	Commit  string   `json:"commit"`
	Latest  *bool    `json:"latest"` // null when not checked: unlocked, --offline or the fetch failed
	Targets []string `json:"targets"`
}

// listRows returns the manifest skills of s, then one row per unmanaged folder
// name. latest reports whether a manifest skill is at its upstream tree.
func listRows(s state, latest func(name string) (*bool, error)) ([]listRow, error) {
	names := make([]string, 0, len(s.m.Skills))
	for n := range s.m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	rows := []listRow{}
	for _, n := range names {
		r := listRow{Name: n, Source: s.m.Skills[n].Source, Managed: true, Commit: s.l.Skills[n].Commit, Targets: s.m.TargetsFor(n)}
		var err error
		if r.Latest, err = latest(n); err != nil {
			return nil, err
		}
		if r.Targets == nil {
			r.Targets = []string{}
		}
		rows = append(rows, r)
	}
	um, err := status.UnmanagedRows(s.m, s.loc.Scope, s.loc.Root, s.home)
	if err != nil {
		return nil, err
	}
	byName := map[string][]string{}
	for _, u := range um {
		byName[u.Skill] = append(byName[u.Skill], u.Target)
	}
	unmanaged := make([]string, 0, len(byName))
	for n := range byName {
		unmanaged = append(unmanaged, n)
	}
	sort.Strings(unmanaged)
	for _, n := range unmanaged {
		rows = append(rows, listRow{Name: n, Source: "unmanaged", Targets: byName[n]})
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
	var asJSON, all, offline bool
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
			states, err := loadScopes(d, scopes, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			rows := []listRow{}
			warned := map[string]bool{} // one fetch warning per URL across scopes
			for _, s := range states {
				sd := d
				sd.Global = isGlobal(s) // the fetcher finds a same-repo source through the scope
				in := status.Input{
					Manifest: s.m, Lock: s.l, Offline: offline,
					Fetcher: &status.GitFetcher{Ctx: cmd.Context(), Open: func(ctx context.Context, sp source.Spec) (*gitx.Repo, error) {
						return install.OpenFetch(ctx, sd, sp)
					}},
					SourceOptions: source.Options{AllowFile: allowFileScheme},
					Warn:          cmd.ErrOrStderr(),
				}
				rs, err := listRows(s, func(name string) (*bool, error) {
					outdated, checked, err := status.CheckLatest(cmd.Context(), in, name, warned)
					if err != nil || !checked {
						return nil, err
					}
					upToDate := !outdated
					return &upToDate, nil
				})
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
			t := render.Table{Header: []string{"NAME", "SOURCE", "MANAGED", "LATEST", "CLAUDE", "COPILOT"}}
			if all {
				t.Header = append([]string{"SCOPE"}, t.Header...)
			}
			for _, r := range rows {
				managed, src := r.Managed, r.Source
				if !managed {
					src = "-"
				}
				in := func(tg string) string {
					if slices.Contains(r.Targets, tg) {
						return "yes"
					}
					return "-"
				}
				cells := []string{r.Name, src, yesNo(&managed), yesNo(r.Latest), in("claude"), in("copilot")}
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
	cmd.Flags().BoolVar(&offline, "offline", false, "do not fetch; leave LATEST unknown")
	return cmd
}

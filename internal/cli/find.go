package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/find"
	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/install"
	"github.com/rojcarranza/cali/internal/loader"
	"github.com/rojcarranza/cali/internal/render"
	"github.com/rojcarranza/cali/internal/source"
)

func newFindCmd(out io.Writer, global *bool) *cobra.Command {
	var asJSON, offline bool
	cmd := &cobra.Command{
		Use:   "find <query>",
		Short: "Search the skills of the configured sources and installed skills",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(args[0]) == "" {
				return errors.New("find: the query is empty")
			}
			d, err := deps(out)
			if err != nil {
				return err
			}
			// Search the given scope; without -g or --manifest, the global one too.
			scopes := []bool{*global}
			if !*global && manifestPath == "" {
				scopes = append(scopes, true)
			}
			type src struct {
				spec   source.Spec
				global bool // OpenFetch finds a same-repo source through the scope
			}
			seen := map[string]bool{}
			var urls []src
			states, err := loadScopes(d, scopes, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			for _, s := range states {
				g := isGlobal(s)
				specs := append([]string{}, s.m.Sources...)
				for _, sk := range s.m.Skills {
					specs = append(specs, sk.Source)
				}
				for _, raw := range specs {
					sp, err := source.ParseWith(raw, source.Options{AllowFile: allowFileScheme})
					if err != nil {
						continue // the manifest was validated on load
					}
					key := sp.URL
					if sp.URL == source.Here {
						key += scopeName(g)
					}
					if seen[key] {
						continue
					}
					seen[key] = true
					urls = append(urls, src{sp, g})
				}
			}
			sort.Slice(urls, func(i, j int) bool { return urls[i].spec.URL < urls[j].spec.URL })

			warn := cmd.ErrOrStderr()
			stop := func() {}
			const wait = "cali is fetching skills…"
			if !asJSON {
				if style, ok := loaderStyle(warn, wait); ok {
					l := loader.Start(warn, wait, 200*time.Millisecond, style)
					stop, warn = l.Stop, l
				}
			}
			hits := []find.Hit{}
			for _, u := range urls {
				sp := u.spec
				d.Global = u.global
				local := sp.URL == source.Here // read in place, never fetched: no mirror
				if offline && !local && !gitx.MirrorExists(d.Dirs.Cache, sp) {
					_, _ = fmt.Fprintf(warn, "warning: %s: no local mirror (run without --offline)\n", sp.URL)
					continue
				}
				var r *gitx.Repo
				if offline && !local {
					if r, err = gitx.Open(d.Dirs.Cache, sp); err == nil {
						r.SetOffline(true)
					}
				} else {
					r, err = install.OpenFetch(cmd.Context(), d, sp)
				}
				if err == nil {
					var h []find.Hit
					if h, err = find.Search([]*gitx.Repo{r}, args[0]); err == nil {
						hits = append(hits, h...)
					}
				}
				if err != nil {
					_, _ = fmt.Fprintf(warn, "warning: %s: %s\n", sp.URL, err)
				}
			}
			stop()
			sort.SliceStable(hits, func(i, j int) bool {
				if hits[i].Name != hits[j].Name {
					return hits[i].Name < hits[j].Name
				}
				return hits[i].Source < hits[j].Source
			})

			mode := render.ModeFor(isTTY(out), asJSON)
			if mode == render.JSON {
				type row struct {
					Name        string `json:"name"`
					Source      string `json:"source"`
					Path        string `json:"path"`
					Description string `json:"description"`
				}
				rows := []row{}
				for _, h := range hits {
					rows = append(rows, row{h.Name, h.Source, h.Path, h.Description})
				}
				return render.WriteJSON(out, struct {
					Hits []row `json:"hits"`
				}{rows})
			}
			t := render.Table{Header: []string{"NAME", "SOURCE", "PATH", "DESCRIPTION"}}
			for _, h := range hits {
				t.Rows = append(t.Rows, []string{h.Name, h.Source, h.Path, h.Description})
			}
			t.Render(out, mode)
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&offline, "offline", false, "do not fetch; search what the mirrors already have")
	return cmd
}

// loaderStyle picks the loader for msg on w; ok is false when w is not a
// terminal or is too narrow for the loader's line.
func loaderStyle(w io.Writer, msg string) (loader.Style, bool) {
	f, ok := w.(*os.File)
	if !ok || !isTTY(w) {
		return loader.Snowdrift, false
	}
	if v := os.Getenv("CALI_LOADER"); v != "" && !strings.EqualFold(v, "princess") && !strings.EqualFold(v, "snowdrift") {
		_, _ = fmt.Fprintf(w, "warning: CALI_LOADER=%q is not princess or snowdrift; using snowdrift\n", v)
	}
	cols, _, err := term.GetSize(f.Fd())
	return pickLoader(cols, err == nil, msg)
}

// pickLoader is loaderStyle for a terminal cols wide (sized false: width
// unknown). No loader is drawn when the width is unknown, since a wrapped line
// cannot be redrawn, or under TERM=dumb, which may print \r literally.
// CALI_LOADER=princess (any case) asks for the princess; she is drawn only on a
// color terminal that understands cursor movement and fits her rows, and
// Snowdrift stands in otherwise.
func pickLoader(cols int, sized bool, msg string) (loader.Style, bool) {
	if !sized || os.Getenv("TERM") == "dumb" {
		return loader.Snowdrift, false
	}
	// shortcut: Windows consoles may lack VT processing, so no princess there; enable VT mode to add her.
	if strings.EqualFold(os.Getenv("CALI_LOADER"), "princess") && os.Getenv("NO_COLOR") == "" &&
		runtime.GOOS != "windows" && cols > loader.Width(msg, loader.Princess) {
		return loader.Princess, true
	}
	return loader.Snowdrift, cols > loader.Width(msg, loader.Snowdrift)
}

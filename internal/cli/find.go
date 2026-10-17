package cli

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rojcarranza/cali/internal/find"
	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/install"
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
			s, err := loadFrom(d, *global)
			if err != nil {
				return err
			}
			specs := append([]string{}, s.m.Sources...)
			for _, sk := range s.m.Skills {
				specs = append(specs, sk.Source)
			}
			seen := map[string]bool{}
			var urls []source.Spec
			for _, raw := range specs {
				sp, err := source.ParseWith(raw, source.Options{AllowFile: allowFileScheme})
				if err != nil || seen[sp.URL] {
					continue // the manifest was validated on load
				}
				seen[sp.URL] = true
				urls = append(urls, sp)
			}
			sort.Slice(urls, func(i, j int) bool { return urls[i].URL < urls[j].URL })

			warn := cmd.ErrOrStderr()
			hits := []find.Hit{}
			for _, sp := range urls {
				if offline && !gitx.MirrorExists(d.Dirs.Cache, sp) {
					_, _ = fmt.Fprintf(warn, "warning: %s: no local mirror (run without --offline)\n", sp.URL)
					continue
				}
				var r *gitx.Repo
				if offline {
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

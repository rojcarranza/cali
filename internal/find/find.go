// Package find searches the skills of already-fetched git mirrors. It scans
// only the repositories it is given; there is no registry (design §10).
package find

import (
	"sort"
	"strings"

	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/skillfs"
)

// maxDepth is how deep below the repository root a SKILL.md is looked for.
const maxDepth = 4

// Hit is one skill whose name or description contains the query.
type Hit struct{ Source, Path, Name, Description string }

// Search returns the skills at the default branch of each repo whose
// frontmatter name or description contains query (case-insensitive), sorted
// by name, then source, then path. A SKILL.md that does not parse is skipped.
// The first repo that cannot be resolved or listed ends the search with an
// error; callers that want to continue past one repo call Search per repo.
func Search(repos []*gitx.Repo, query string) ([]Hit, error) {
	q := strings.ToLower(query)
	var hits []Hit
	for _, r := range repos {
		commit, err := r.Resolve("")
		if err != nil {
			return nil, err
		}
		files, err := r.ReadSkillFiles(commit, maxDepth)
		if err != nil {
			return nil, err
		}
		// An oversize or (offline) missing SKILL.md is absent from files.
		for d, data := range files {
			fm, err := skillfs.ParseFrontmatter(data)
			if err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(fm.Name), q) || strings.Contains(strings.ToLower(fm.Description), q) {
				hits = append(hits, Hit{r.URL(), d, fm.Name, fm.Description})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Path < b.Path
	})
	return hits, nil
}

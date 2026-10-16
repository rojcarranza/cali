package find

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/gitx"
	"github.com/rojcarranza/cali/internal/source"
)

// repo commits files (path -> content) into a new git repo, fetches it into a
// fresh mirror and returns the mirror.
func repo(t *testing.T, files map[string]string) *gitx.Repo {
	t.Helper()
	return repoFilter(t, files, false)
}

// repoFilter is repo; with filter the source allows blob:none, so the mirror
// holds no blobs after Fetch.
func repoFilter(t *testing.T, files map[string]string, filter bool) *gitx.Repo {
	t.Helper()
	empty := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", empty)
	dir := t.TempDir()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}, {"commit", "-q", "-m", "one"}, {"config", "uploadpack.allowFilter", fmt.Sprint(filter)}} {
		cmd := exec.Command("git", append([]string{"-c", "user.name=T", "-c", "user.email=t@example.invalid"}, a...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", a, err, out)
		}
	}
	s, err := source.ParseWith("file://"+filepath.ToSlash(dir), source.Options{AllowFile: true})
	if err != nil {
		t.Fatal(err)
	}
	r, err := gitx.Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Fetch(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	return r
}

func skill(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody\n"
}

func TestSearchMatchesDescription(t *testing.T) {
	r := repo(t, map[string]string{
		"skills/a/SKILL.md": skill("alpha", "Reviews Terraform plans"),
		"skills/b/SKILL.md": skill("beta", "Writes poems"),
		"skills/c/SKILL.md": "no frontmatter terraform",
	})
	hits, err := Search([]*gitx.Repo{r}, "TERRAFORM")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "alpha" || hits[0].Path != "skills/a" ||
		hits[0].Description != "Reviews Terraform plans" || hits[0].Source != r.URL() {
		t.Fatalf("hits = %+v", hits)
	}
	if hits, _ = Search([]*gitx.Repo{r}, "BET"); len(hits) != 1 || hits[0].Name != "beta" {
		t.Fatalf("name match: hits = %+v", hits)
	}
}

func TestSearchDepthLimit(t *testing.T) {
	r := repo(t, map[string]string{
		"a/b/c/d/SKILL.md":   skill("deep4", "match"),
		"a/b/c/d/e/SKILL.md": skill("deep5", "match"),
		"SKILL.md":           skill("root", "match"),
	})
	hits, err := Search([]*gitx.Repo{r}, "match")
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, h := range hits {
		names = append(names, h.Name)
	}
	if got := strings.Join(names, ","); got != "deep4,root" {
		t.Fatalf("names = %q", got)
	}
}

func TestSearchNoRegistry(t *testing.T) {
	listed := repo(t, map[string]string{"s/SKILL.md": skill("one", "common word")})
	other := repo(t, map[string]string{"s/SKILL.md": skill("two", "common word")})
	_ = other
	hits, err := Search([]*gitx.Repo{listed}, "common")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Name != "one" || hits[0].Source != listed.URL() {
		t.Fatalf("hits = %+v", hits)
	}
}

func TestSearchOfflineSkipsMissingBlob(t *testing.T) {
	r := repoFilter(t, map[string]string{"s/SKILL.md": skill("one", "common word")}, true)
	r.SetOffline(true)
	hits, err := Search([]*gitx.Repo{r}, "common")
	if err != nil || len(hits) != 0 {
		t.Fatalf("offline: hits=%+v err=%v", hits, err)
	}
	r.SetOffline(false)
	if hits, err = Search([]*gitx.Repo{r}, "common"); err != nil || len(hits) != 1 {
		t.Fatalf("online (proves the blob was only missing): hits=%+v err=%v", hits, err)
	}
}

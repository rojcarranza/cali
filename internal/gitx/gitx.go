// Package gitx talks to the system git (ADR 0001): bare mirrors, ref
// resolution and blob-level export of a skill folder without any checkout.
package gitx

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
)

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Repo is a bare mirror of one source. Dir is the bare repository directory.
type Repo struct {
	Dir string
	url string
	env []string // extra env of the last Fetch, reused for lazy blob fetches
}

// Open returns the mirror for s at <cacheDir>/git/<key>.git, creating the
// bare repository if needed. It does not touch the network.
func Open(cacheDir string, s source.Spec) (*Repo, error) {
	key := strings.ReplaceAll(s.Key(), ":", "_")
	dir := filepath.Join(cacheDir, "git", filepath.FromSlash(key)+".git")
	r := &Repo{Dir: dir, url: s.URL}
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if _, err := r.git(context.Background(), nil, "init", "--bare", "-q", dir); err != nil {
			return nil, err
		}
	}
	if _, err := r.git(context.Background(), nil, "--git-dir", dir, "config", "remote.origin.url", s.URL); err != nil {
		return nil, err
	}
	return r, nil
}

// git runs git with core.autocrlf=false and prompts disabled. The mirror is
// selected with --git-dir unless args already starts with "init".
func (r *Repo) git(ctx context.Context, env []string, args ...string) ([]byte, error) {
	full := []string{"-c", "core.autocrlf=false"}
	if len(args) > 0 && args[0] != "init" && args[0] != "--git-dir" {
		full = append(full, "--git-dir", r.Dir)
	}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Env = append(cmd.Env, env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := scrub(strings.TrimSpace(stderr.String()), env)
		return nil, fmt.Errorf("git %s: %w: %s", gitVerb(args), err, msg)
	}
	return stdout.Bytes(), nil
}

func gitVerb(args []string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == "--git-dir" {
			i++
			continue
		}
		return args[i]
	}
	return ""
}

// scrub removes credential material in env from s.
func scrub(s string, env []string) string {
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(k, "GIT_CONFIG_VALUE_") {
			continue
		}
		s = strings.ReplaceAll(s, v, "***")
		if b64, ok := strings.CutPrefix(v, "AUTHORIZATION: basic "); ok {
			s = strings.ReplaceAll(s, b64, "***")
			if raw, err := base64.StdEncoding.DecodeString(b64); err == nil {
				if tok, ok := strings.CutPrefix(string(raw), "x-access-token:"); ok && tok != "" {
					s = strings.ReplaceAll(s, tok, "***")
				}
			}
		}
	}
	return s
}

// Fetch updates the mirror's branches, tags and default branch. It first
// tries a blob-less fetch (--filter=blob:none) and falls back to a full one
// if the server refuses the filter. env is extra environment, e.g. AuthEnv.
func (r *Repo) Fetch(ctx context.Context, env []string) error {
	r.env = env
	specs := []string{
		"+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*",
		"+HEAD:refs/cali/head",
	}
	base := []string{"fetch", "-q", "--no-tags", "--prune"}
	filtered := append(append(append([]string{}, base...), "--filter=blob:none", "origin"), specs...)
	if _, err := r.git(ctx, env, filtered...); err == nil {
		return nil
	} else if ctx.Err() != nil {
		return err
	}
	plain := append(append(append([]string{}, base...), "origin"), specs...)
	_, err := r.git(ctx, env, plain...)
	return err
}

// HasCommit reports whether commit exists in the mirror.
func (r *Repo) HasCommit(commit string) bool {
	if !hex40.MatchString(commit) {
		return false
	}
	_, err := r.git(context.Background(), r.env, "cat-file", "-e", commit+"^{commit}")
	return err == nil
}

// Resolve turns ref into a full commit SHA. An empty ref is the remote
// default branch. A full 40-hex commit is returned as is if present;
// otherwise ref is looked up as a branch, then a tag.
func (r *Repo) Resolve(ref string) (string, error) {
	if ref == "" {
		return r.revParse("refs/cali/head", "default branch")
	}
	if hex40.MatchString(ref) && r.HasCommit(ref) {
		return ref, nil
	}
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\n\x00") {
		return "", fmt.Errorf("invalid ref %q", ref)
	}
	for _, full := range []string{"refs/heads/" + ref, "refs/tags/" + ref} {
		if c, err := r.revParse(full, ref); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("ref %q not found in %s", ref, r.url)
}

func (r *Repo) revParse(rev, what string) (string, error) {
	out, err := r.git(context.Background(), r.env, "rev-parse", "--verify", "-q", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve %s in %s", what, r.url)
	}
	return strings.TrimSpace(string(out)), nil
}

// TreeSHA returns the tree SHA of path (empty means the root) at commit.
func (r *Repo) TreeSHA(commit, path string) (string, error) {
	if !hex40.MatchString(commit) {
		return "", fmt.Errorf("invalid commit %q", commit)
	}
	rev := commit + "^{tree}"
	if path = strings.Trim(path, "/"); path != "" {
		rev = commit + ":" + path
	}
	out, err := r.git(context.Background(), r.env, "rev-parse", "--verify", "-q", "--end-of-options", rev)
	if err != nil {
		return "", fmt.Errorf("path %q not found at commit %s", path, commit)
	}
	sha := strings.TrimSpace(string(out))
	typ, err := r.git(context.Background(), r.env, "cat-file", "-t", sha)
	if err != nil || strings.TrimSpace(string(typ)) != "tree" {
		return "", fmt.Errorf("path %q at commit %s is not a directory", path, commit)
	}
	return sha, nil
}

// Export reads the folder path at commit into a Tree straight from git
// blobs, so no checkout filter can alter the bytes. Symlinks and submodules
// are errors naming the path. The result is validated by skillfs.Validate.
func (r *Repo) Export(commit, path string) (skillfs.Tree, error) {
	tree, err := r.TreeSHA(commit, path)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	out, err := r.git(ctx, r.env, "ls-tree", "-r", "-z", tree)
	if err != nil {
		return nil, err
	}
	var t skillfs.Tree
	for _, rec := range bytes.Split(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		meta, name, ok := bytes.Cut(rec, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("unexpected ls-tree entry %q", rec)
		}
		mode, sha := fields[0], fields[2]
		p := string(name)
		switch mode {
		case "120000":
			return nil, fmt.Errorf("%s: symlinks are not allowed in a skill", p)
		case "160000":
			return nil, fmt.Errorf("%s: submodules are not allowed in a skill", p)
		case "100644", "100755":
		default:
			return nil, fmt.Errorf("%s: unsupported file mode %s", p, mode)
		}
		data, err := r.git(ctx, r.env, "cat-file", "blob", sha)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		t = append(t, skillfs.File{Path: p, Exec: mode == "100755", Data: data})
	}
	if len(t) == 0 {
		return nil, errors.New("no files at " + fmt.Sprintf("%q", path))
	}
	// git ls-tree emits entries in byte order, which is the sorted Path order.
	if err := skillfs.Validate(t); err != nil {
		return nil, err
	}
	return t, nil
}

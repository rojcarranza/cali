// Package gitx talks to the system git (ADR 0001): bare mirrors, ref
// resolution and blob-level export of a skill folder without any checkout.
package gitx

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/rojcarranza/cali/internal/skillfs"
	"github.com/rojcarranza/cali/internal/source"
)

var hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// ErrNotFound is wrapped by Resolve and TreeSHA when the ref, or the folder at
// the commit, does not exist upstream (as opposed to an invalid input).
var ErrNotFound = errors.New("not found upstream")

// maxStderr caps the git stderr text that is placed into an error.
const maxStderr = 4096

// limits mirrors the skillfs limits; it is a variable so tests can shrink it.
var limits = struct {
	files      int
	file, tree int64
}{skillfs.MaxFiles, skillfs.MaxFileSize, skillfs.MaxTreeSize}

// Repo is a bare mirror of one source. Dir is the bare repository directory.
type Repo struct {
	Dir   string
	url   string
	proto string   // value of GIT_ALLOW_PROTOCOL
	env   []string // extra env of the last Fetch, reused for lazy blob fetches
	// offline forbids lazy blob fetches (GIT_NO_LAZY_FETCH=1).
	offline bool
}

// SetOffline makes every later git call on r refuse to fetch missing objects.
func (r *Repo) SetOffline(v bool) { r.offline = v }

// mirrorDir returns the cache root and the mirror directory of s.
func mirrorDir(cacheDir string, s source.Spec) (root, dir string) {
	root = filepath.Join(cacheDir, "git")
	key := strings.ReplaceAll(s.Key(), ":", "_")
	return root, filepath.Join(root, filepath.FromSlash(key)+".git")
}

// MirrorExists reports whether Open has created the mirror of s before.
func MirrorExists(cacheDir string, s source.Spec) bool {
	_, dir := mirrorDir(cacheDir, s)
	_, err := os.Stat(filepath.Join(dir, "HEAD"))
	return err == nil
}

// Open returns the mirror for s at <cacheDir>/git/<key>.git, creating the
// bare repository if needed. The mirror is configured as a blob-less
// (promisor) clone of s.URL. Open does not touch the network.
func Open(cacheDir string, s source.Spec) (*Repo, error) {
	root, dir := mirrorDir(cacheDir, s)
	key := strings.ReplaceAll(s.Key(), ":", "_")
	if rel, err := filepath.Rel(root, dir); err != nil || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return nil, fmt.Errorf("mirror path for %q escapes the cache", key)
	}
	r := &Repo{Dir: dir, url: s.URL, proto: "https"}
	if strings.HasPrefix(s.URL, "file://") {
		r.proto = "https:file"
	}
	ctx := context.Background()
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		if _, err := r.run(ctx, nil, "init", "--bare", "-q", dir); err != nil {
			return nil, err
		}
	}
	// Read the config once and write only what is missing or different, so an
	// existing mirror costs one git call and a half-configured one is repaired.
	out, err := r.git(ctx, nil, "config", "--local", "-z", "--list")
	if err != nil {
		return nil, err
	}
	have := map[string]string{}
	for _, rec := range strings.Split(string(out), "\x00") {
		k, v, _ := strings.Cut(rec, "\n")
		have[k] = v
	}
	for _, kv := range [][2]string{
		{"core.repositoryformatversion", "1"},
		{"extensions.partialclone", "origin"},
		{"remote.origin.url", s.URL},
		{"remote.origin.promisor", "true"},
		{"remote.origin.partialclonefilter", "blob:none"},
	} {
		if v, ok := have[kv[0]]; ok && v == kv[1] {
			continue
		}
		if _, err := r.git(ctx, nil, "config", kv[0], kv[1]); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// environ builds the child environment: base without any inherited
// GIT_CONFIG_COUNT/KEY_n/VALUE_n or GIT_ALLOW_PROTOCOL, then prompts off,
// the protocol allow-list and the extra entries.
func environ(base []string, proto string, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra)+2)
	for _, e := range base {
		k, _, _ := strings.Cut(e, "=")
		if k == "GIT_CONFIG_COUNT" || k == "GIT_ALLOW_PROTOCOL" ||
			strings.HasPrefix(k, "GIT_CONFIG_KEY_") || strings.HasPrefix(k, "GIT_CONFIG_VALUE_") {
			continue
		}
		out = append(out, e)
	}
	out = append(out, "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL="+proto)
	return append(out, extra...)
}

// command builds a git invocation with the fixed hardening flags. args are
// used as given (the caller adds --git-dir where needed).
func (r *Repo) command(ctx context.Context, env []string, args ...string) *exec.Cmd {
	full := []string{"-c", "core.autocrlf=false", "-c", "http.followRedirects=false", "-c", "protocol.ext.allow=never"}
	full = append(full, args...)
	if r.offline {
		env = append(append([]string{}, env...), "GIT_NO_LAZY_FETCH=1")
	}
	cmd := exec.CommandContext(ctx, "git", full...)
	cmd.Env = environ(os.Environ(), r.proto, env)
	return cmd
}

// run executes git without selecting the mirror.
func (r *Repo) run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := r.command(ctx, env, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, sanitize(stderr.String(), env))
	}
	return stdout.Bytes(), nil
}

// git executes git against the mirror.
func (r *Repo) git(ctx context.Context, env []string, args ...string) ([]byte, error) {
	return r.run(ctx, env, append([]string{"--git-dir", r.Dir}, args...)...)
}

// sanitize makes git stderr safe to put into an error: credentials removed,
// control characters (C0, C1, ESC, DEL) dropped, size capped.
func sanitize(s string, env []string) string {
	s = scrub(strings.TrimSpace(s), env)
	s = strings.ToValidUTF8(s, "?")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	if len(s) > maxStderr {
		s = strings.ToValidUTF8(s[:maxStderr], "") + "..."
	}
	return s
}

// scrub removes credential material in env from s.
func scrub(s string, env []string) string {
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		if !strings.HasPrefix(k, "GIT_CONFIG_VALUE_") || v == "" {
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
// (--no-filter) only when the server rejects that filter. A server without
// filter support needs no fallback: git warns and fetches everything.
// env is extra environment, e.g. AuthEnv; it is kept for later lazy blob
// fetches, so Fetch must run before Export.
func (r *Repo) Fetch(ctx context.Context, env []string) error {
	r.env = env
	specs := []string{
		"+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*",
		"+HEAD:refs/cali/head",
	}
	base := []string{"fetch", "-q", "--no-tags", "--prune"}
	build := func(filter string) []string {
		a := append(append([]string{}, base...), filter, "origin")
		return append(a, specs...)
	}
	_, err := r.git(ctx, env, build("--filter=blob:none")...)
	if err == nil || ctx.Err() != nil || !strings.Contains(err.Error(), "filter 'blob:none' not supported") {
		return err
	}
	if _, err2 := r.git(ctx, env, build("--no-filter")...); err2 != nil {
		return errors.Join(err, err2)
	}
	return nil
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
// otherwise ref is looked up as a branch, then a tag. Revision expressions
// (main~1, v1^, a..b, @{u}) are rejected.
func (r *Repo) Resolve(ref string) (string, error) {
	if ref == "" {
		return r.revParse("refs/cali/head", "default branch")
	}
	if hex40.MatchString(ref) && r.HasCommit(ref) {
		return ref, nil
	}
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\n\x00~^:\\?*[") ||
		strings.Contains(ref, "@{") || strings.Contains(ref, "..") {
		return "", fmt.Errorf("invalid ref %q", ref)
	}
	for _, full := range []string{"refs/heads/" + ref, "refs/tags/" + ref} {
		if c, err := r.revParse(full, ref); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("ref %q not found in %s: %w", ref, r.url, ErrNotFound)
}

func (r *Repo) revParse(rev, what string) (string, error) {
	out, err := r.git(context.Background(), r.env, "rev-parse", "--verify", "-q", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q in %s", what, r.url)
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
		return "", fmt.Errorf("path %q not found at commit %s: %w", path, commit, ErrNotFound)
	}
	sha := strings.TrimSpace(string(out))
	typ, err := r.git(context.Background(), r.env, "cat-file", "-t", sha)
	if err != nil || strings.TrimSpace(string(typ)) != "tree" {
		return "", fmt.Errorf("path %q at commit %s is not a directory: %w", path, commit, ErrNotFound)
	}
	return sha, nil
}

type entry struct {
	path string
	sha  string
	size int64
	exec bool
}

// Export reads the folder whose tree SHA is tree (from TreeSHA) into a Tree
// straight from git blobs, so no checkout filter can alter the bytes. Symlinks and submodules
// are errors naming the path. File count and sizes are checked against the
// skillfs limits from the tree listing before any blob is read into memory,
// and the result is validated by skillfs.Validate. Blobs the blob-less
// mirror lacks are downloaded first in one fetch (ls-tree -l needs them for
// the sizes); only the file count is checked before that download.
//
// Fetch must have run on this Repo first: the mirror is blob-less, and the
// auth environment given to Fetch is what lets git download blobs lazily.
func (r *Repo) Export(tree string) (skillfs.Tree, error) {
	if !hex40.MatchString(tree) {
		return nil, fmt.Errorf("invalid tree %q", tree)
	}
	ctx := context.Background()
	if !r.offline {
		if err := r.fetchMissing(ctx, tree); err != nil {
			return nil, err
		}
	}
	out, err := r.git(ctx, r.env, "ls-tree", "-r", "-z", "-l", tree)
	if err != nil {
		return nil, err
	}
	var entries []entry
	var total int64
	for _, rec := range bytes.Split(out, []byte{0}) {
		if len(rec) == 0 {
			continue
		}
		meta, name, ok := bytes.Cut(rec, []byte{'\t'})
		fields := strings.Fields(string(meta))
		if !ok || len(fields) != 4 {
			return nil, fmt.Errorf("unexpected ls-tree entry %q", rec)
		}
		mode, sha, p := fields[0], fields[2], string(name)
		switch mode {
		case "120000":
			return nil, fmt.Errorf("%q: symlinks are not allowed in a skill", p)
		case "160000":
			return nil, fmt.Errorf("%q: submodules are not allowed in a skill", p)
		case "100644", "100755":
		default:
			return nil, fmt.Errorf("%q: unsupported file mode %s", p, mode)
		}
		size, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q: bad size in tree listing", p)
		}
		if len(entries)+1 > limits.files {
			return nil, fmt.Errorf("skill has more than %d files", limits.files)
		}
		if size > limits.file {
			return nil, fmt.Errorf("%q: file is %d bytes, limit is %d", p, size, limits.file)
		}
		if total += size; total > limits.tree {
			return nil, fmt.Errorf("skill is larger than %d bytes", limits.tree)
		}
		entries = append(entries, entry{p, sha, size, mode == "100755"})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no files in tree %s", tree)
	}
	t, err := r.readBlobs(ctx, entries)
	if err != nil {
		return nil, err
	}
	// git ls-tree emits entries in byte order, which is the sorted Path order.
	if err := skillfs.Validate(t); err != nil {
		return nil, err
	}
	return t, nil
}

// fetchMissing downloads every blob of tree the blob-less mirror lacks in one
// fetch (the command git itself runs for a lazy fetch), instead of the one
// lazy fetch per blob that ls-tree -l would trigger.
func (r *Repo) fetchMissing(ctx context.Context, tree string) error {
	out, err := r.git(ctx, r.env, "rev-list", "--objects", "--missing=print", tree)
	if err != nil {
		return err
	}
	var want strings.Builder
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		if oid, ok := strings.CutPrefix(line, "?"); ok {
			want.WriteString(oid + "\n")
			n++
		}
	}
	// Every missing object is a distinct blob, so n is at most the file count.
	if n > limits.files {
		return fmt.Errorf("skill has more than %d files", limits.files)
	}
	if n == 0 {
		return nil
	}
	cmd := r.command(ctx, r.env, "--git-dir", r.Dir, "-c", "fetch.negotiationAlgorithm=noop",
		"fetch", "-q", "--no-tags", "--no-write-fetch-head", "--recurse-submodules=no", "--filter=blob:none", "--stdin", "origin")
	cmd.Stdin = strings.NewReader(want.String())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git fetch of missing blobs: %w: %s", err, sanitize(stderr.String(), r.env))
	}
	return nil
}

// readBlobs reads every entry through one git cat-file --batch process. Each
// read is bounded by the size taken from the tree listing.
func (r *Repo) readBlobs(ctx context.Context, entries []entry) (skillfs.Tree, error) {
	cmd := r.command(ctx, r.env, "--git-dir", r.Dir, "cat-file", "--batch")
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	finish := func(e error) (skillfs.Tree, error) {
		_ = in.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("git cat-file: %w: %s", e, sanitize(stderr.String(), r.env))
	}
	br := bufio.NewReader(outPipe)
	t := make(skillfs.Tree, 0, len(entries))
	for _, e := range entries {
		if _, err := io.WriteString(in, e.sha+"\n"); err != nil {
			return finish(err)
		}
		line, err := br.ReadString('\n')
		if err != nil {
			return finish(fmt.Errorf("%q: %w", e.path, err))
		}
		f := strings.Fields(line)
		if len(f) != 3 || f[0] != e.sha || f[1] != "blob" {
			return finish(fmt.Errorf("%q: unexpected cat-file reply %q", e.path, strings.TrimSpace(line)))
		}
		if n, err := strconv.ParseInt(f[2], 10, 64); err != nil || n != e.size {
			return finish(fmt.Errorf("%q: blob size differs from tree listing", e.path))
		}
		data := make([]byte, e.size)
		if _, err := io.ReadFull(br, data); err != nil {
			return finish(fmt.Errorf("%q: %w", e.path, err))
		}
		if b, err := br.ReadByte(); err != nil || b != '\n' {
			return finish(errors.New("malformed cat-file output"))
		}
		t = append(t, skillfs.File{Path: e.path, Exec: e.exec, Data: data})
	}
	_ = in.Close()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("git cat-file: %w: %s", err, sanitize(stderr.String(), r.env))
	}
	return t, nil
}

// ListSkillDirs returns the directories at commit that hold a regular file
// named SKILL.md, at most maxDepth path segments deep ("" is the repository
// root, depth 0). The result is sorted. Only tree listings are read, so no
// blob is downloaded.
func (r *Repo) ListSkillDirs(commit string, maxDepth int) ([]string, error) {
	if !hex40.MatchString(commit) {
		return nil, fmt.Errorf("invalid commit %q", commit)
	}
	out, err := r.git(context.Background(), r.env, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, rec := range bytes.Split(out, []byte{0}) {
		meta, name, ok := bytes.Cut(rec, []byte{'\t'})
		if !ok {
			continue
		}
		fields := strings.Fields(string(meta))
		if len(fields) != 3 || fields[1] != "blob" || (fields[0] != "100644" && fields[0] != "100755") {
			continue
		}
		p := string(name)
		var dir string
		switch {
		case p == "SKILL.md":
		case strings.HasSuffix(p, "/SKILL.md"):
			dir = strings.TrimSuffix(p, "/SKILL.md")
		default:
			continue
		}
		if dir != "" && strings.Count(dir, "/")+1 > maxDepth {
			continue
		}
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// URL returns the source URL the mirror was opened for.
func (r *Repo) URL() string { return r.url }

// ReadFile returns the regular file at path in commit. The size is taken from
// the tree listing and a file over the skillfs.MaxFileSize limit is refused
// before its blob is read.
func (r *Repo) ReadFile(commit, path string) ([]byte, error) {
	if !hex40.MatchString(commit) {
		return nil, fmt.Errorf("invalid commit %q", commit)
	}
	ctx := context.Background()
	out, err := r.git(ctx, r.env, "ls-tree", "-z", "-l", commit, "--", path)
	if err != nil {
		return nil, err
	}
	meta, name, ok := bytes.Cut(bytes.TrimSuffix(out, []byte{0}), []byte{'\t'})
	fields := strings.Fields(string(meta))
	if !ok || len(fields) != 4 || string(name) != path || fields[1] != "blob" ||
		(fields[0] != "100644" && fields[0] != "100755") {
		return nil, fmt.Errorf("%q is not a regular file at commit %s", path, commit)
	}
	size, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%q: bad size in tree listing", path)
	}
	if size > limits.file {
		return nil, fmt.Errorf("%q: file is %d bytes, limit is %d", path, size, limits.file)
	}
	return r.git(ctx, r.env, "cat-file", "blob", fields[2])
}

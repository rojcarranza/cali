// Package source parses skill source specs such as
// github:owner/repo//path/to/skill@ref.
package source

import (
	"fmt"
	"strings"
)

// Spec is a parsed source spec: a git URL, an optional sub-path inside the
// repository and an optional ref (tag, branch or commit).
type Spec struct {
	URL, Path, Ref string
}

// Options tunes Parse. AllowFile accepts file:// URLs (local git fixtures).
type Options struct{ AllowFile bool }

// Parse parses a source spec with the default options.
func Parse(s string) (Spec, error) { return ParseWith(s, Options{}) }

// ParseWith parses a source spec. Accepted forms are
// github:owner/repo[//path][@ref] and https://host/owner/repo[.git][//path][@ref]
// (plus file:///abs/path[//path][@ref] when o.AllowFile is set).
func ParseWith(s string, o Options) (Spec, error) {
	var scheme, rest string
	switch {
	case strings.HasPrefix(s, "github:"):
		scheme, rest = "github", strings.TrimPrefix(s, "github:")
	case strings.Contains(s, "://"):
		i := strings.Index(s, "://")
		scheme, rest = s[:i], s[i+3:]
	default:
		return Spec{}, fmt.Errorf("source %q: unsupported form, want github:owner/repo or https://host/owner/repo", s)
	}
	switch scheme {
	case "github", "https":
	case "file":
		if !o.AllowFile {
			return Spec{}, fmt.Errorf("source %q: scheme file is not allowed", s)
		}
	default:
		return Spec{}, fmt.Errorf("source %q: scheme %s is not allowed, use github: or https://", s, scheme)
	}

	head, tail, hasPath := strings.Cut(rest, "//")
	if scheme == "https" {
		host, _, _ := strings.Cut(head, "/")
		if strings.Contains(host, "@") {
			return Spec{}, fmt.Errorf("source %q: credentials (user:pass@) in URL are not allowed", s)
		}
	}

	var ref string
	if hasPath {
		if i := strings.LastIndex(tail, "@"); i >= 0 {
			tail, ref = tail[:i], tail[i+1:]
		}
	} else if i := strings.LastIndex(head, "@"); i >= 0 {
		head, ref = head[:i], head[i+1:]
	}

	path := tail
	if strings.HasPrefix(path, "/") {
		return Spec{}, fmt.Errorf("source %q: path %q has a leading /", s, path)
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return Spec{}, fmt.Errorf("source %q: path %q contains a .. segment", s, path)
		}
	}

	spec := Spec{Path: path, Ref: ref}
	switch scheme {
	case "github":
		owner, repo, err := ownerRepo(s, strings.Split(head, "/"))
		if err != nil {
			return Spec{}, err
		}
		spec.URL = "https://github.com/" + owner + "/" + repo
	case "https":
		parts := strings.Split(head, "/")
		if parts[0] == "" {
			return Spec{}, fmt.Errorf("source %q: empty host", s)
		}
		if _, _, err := ownerRepo(s, parts[1:]); err != nil {
			return Spec{}, err
		}
		spec.URL = "https://" + head
	case "file":
		if strings.Trim(head, "/") == "" {
			return Spec{}, fmt.Errorf("source %q: empty file path", s)
		}
		spec.URL = "file://" + head
	}
	return spec, nil
}

func ownerRepo(s string, parts []string) (owner, repo string, err error) {
	if len(parts) > 0 {
		owner = parts[0]
	}
	if owner == "" {
		return "", "", fmt.Errorf("source %q: empty owner", s)
	}
	if len(parts) > 1 {
		repo = strings.TrimSuffix(parts[1], ".git")
	}
	if repo == "" {
		return "", "", fmt.Errorf("source %q: empty repo", s)
	}
	if len(parts) > 2 {
		return "", "", fmt.Errorf("source %q: want exactly owner/repo, got extra segment %q", s, parts[2])
	}
	return owner, parts[1], nil
}

// String returns the canonical form: URL[//Path][@Ref].
func (s Spec) String() string {
	out := s.URL
	if s.Path != "" {
		out += "//" + s.Path
	}
	if s.Ref != "" {
		out += "@" + s.Ref
	}
	return out
}

// Key returns <host>/<owner>/<repo> (no .git suffix), used to name mirror dirs.
// For file:// URLs it is "file/" plus the path.
func (s Spec) Key() string {
	if rest, ok := strings.CutPrefix(s.URL, "file://"); ok {
		return "file/" + strings.Trim(rest, "/")
	}
	rest := strings.TrimPrefix(s.URL, "https://")
	return strings.TrimSuffix(rest, ".git")
}

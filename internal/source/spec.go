// Package source parses skill source specs such as
// github:owner/repo//path/to/skill@ref.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
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
		return Spec{}, reject("unsupported form, want github:owner/repo or https://host/owner/repo")
	}
	switch scheme {
	case "github", "https":
	case "file":
		if !o.AllowFile {
			return Spec{}, reject("scheme file is not allowed")
		}
	default:
		return Spec{}, reject("scheme " + safeScheme(scheme) + " is not allowed, use github: or https://")
	}

	if scheme != "github" {
		authority, _, _ := strings.Cut(rest, "/")
		if strings.Contains(authority, "@") {
			return Spec{}, reject("embedded credentials are not allowed")
		}
	}

	head, tail, hasPath := strings.Cut(rest, "//")

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
		return Spec{}, reject("path has a leading /")
	}
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." {
			return Spec{}, reject("path contains a .. segment")
		}
	}

	spec := Spec{Path: path, Ref: ref}
	switch scheme {
	case "github":
		owner, repo, err := ownerRepo(strings.Split(head, "/"))
		if err != nil {
			return Spec{}, err
		}
		// GitHub ignores case in owner and repo, so fold it: one repo, one source.
		spec.URL = "https://github.com/" + strings.ToLower(owner) + "/" + strings.ToLower(repo)
	case "https":
		parts := strings.Split(head, "/")
		if parts[0] == "" {
			return Spec{}, reject("empty host")
		}
		if !validPart(parts[0], true) {
			return Spec{}, reject("host is invalid")
		}
		owner, repo, err := ownerRepo(parts[1:])
		if err != nil {
			return Spec{}, err
		}
		host := strings.ToLower(parts[0])
		if host == "github.com" {
			owner, repo = strings.ToLower(owner), strings.ToLower(repo)
		}
		spec.URL = "https://" + host + "/" + owner + "/" + repo
	case "file":
		if strings.Trim(head, "/") == "" {
			return Spec{}, reject("empty file path")
		}
		spec.URL = "file://" + head
	}
	return spec, nil
}

// SameURL reports whether lockURL, a source URL from cali.lock, names the
// canonical url. It canonicalises lockURL first, so an entry written before
// a canonical-form change (github.com owner/repo case) still matches.
func SameURL(lockURL, url string, o Options) bool {
	s, err := ParseWith(lockURL, o)
	return err == nil && s.URL == url
}

// reject is the only way ParseWith builds an error. It takes a fixed reason and
// never the input, because the input may carry credentials.
func reject(reason string) error {
	return errors.New("invalid source spec: " + reason)
}

// safeScheme returns scheme if it is made of URL-scheme characters only,
// otherwise a placeholder, so a malformed scheme cannot carry a secret.
func safeScheme(scheme string) string {
	for _, r := range scheme {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+-.", r) {
			return "(invalid)"
		}
	}
	return scheme
}

func ownerRepo(parts []string) (owner, repo string, err error) {
	if len(parts) > 0 {
		owner = parts[0]
	}
	if owner == "" {
		return "", "", reject("empty owner")
	}
	if len(parts) > 1 {
		repo = strings.TrimSuffix(parts[1], ".git")
	}
	if repo == "" {
		return "", "", reject("empty repo")
	}
	if !validPart(owner, false) {
		return "", "", reject("owner is invalid")
	}
	if !validPart(repo, false) {
		return "", "", reject("repo is invalid")
	}
	if len(parts) > 2 {
		if extra := parts[2]; !strings.ContainsAny(extra, "@:") {
			return "", "", reject("want exactly owner/repo, got extra segment " + strconv.Quote(extra))
		}
		return "", "", reject("want exactly owner/repo, got extra segment")
	}
	return owner, repo, nil
}

// validPart reports whether a host, owner or repo is made of [A-Za-z0-9._-]
// only (a host may add :<port>), is not . or .. and does not start with -.
// This keeps Key() inside the mirror cache and out of git's option parsing.
func validPart(p string, host bool) bool {
	if host {
		if h, port, ok := strings.Cut(p, ":"); ok {
			if port == "" || strings.Trim(port, "0123456789") != "" {
				return false
			}
			p = h
		}
	}
	if p == "" || p == "." || p == ".." || p[0] == '-' {
		return false
	}
	for _, r := range p {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-", r) {
			return false
		}
	}
	return true
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
// For file:// URLs it is "file/" plus the first 16 hex chars of the SHA-256
// of the path: short and free of ":" so Windows paths stay under MAX_PATH.
func (s Spec) Key() string {
	if rest, ok := strings.CutPrefix(s.URL, "file://"); ok {
		sum := sha256.Sum256([]byte(rest))
		return "file/" + hex.EncodeToString(sum[:])[:16]
	}
	rest := strings.TrimPrefix(s.URL, "https://")
	return strings.TrimSuffix(rest, ".git")
}

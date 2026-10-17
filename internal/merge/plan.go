// Package merge plans a three-way update of a skill folder: per file, keep the
// local copy, take upstream, use a clean line merge, or report a conflict.
// It never writes conflict markers into a planned result.
package merge

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rojcarranza/cali/internal/skillfs"
)

// Action is what update does with one path.
type Action string

// Actions of a FilePlan.
const (
	KeepLocal    Action = "keep-local"
	TakeUpstream Action = "take-upstream"
	Merged       Action = "merged"
	Conflicted   Action = "conflict" // named so it does not clash with type Conflict
)

// FilePlan is the planned result for one path. Merged holds the new content
// unless Delete is set or Action is Conflict.
type FilePlan struct {
	Path     string
	Action   Action
	Merged   []byte
	Delete   bool
	Exec     bool
	Conflict *Conflict
}

// Conflict carries the three sides as given and, for text, the conflicting hunks.
type Conflict struct {
	Ours, Base, Theirs []byte
	Hunks              []Hunk
}

// Hunk is one conflicting region; OursStart is its 1-based first line in ours.
type Hunk struct {
	OursStart int
	Ours      []string
	Base      []string
	Theirs    []string
}

func take(p string, a Action, f *skillfs.File) FilePlan {
	if f == nil {
		return FilePlan{Path: p, Action: a, Delete: true}
	}
	return FilePlan{Path: p, Action: a, Merged: f.Data, Exec: f.Exec}
}

func data(f *skillfs.File) []byte {
	if f == nil {
		return nil
	}
	return f.Data
}

// Plan returns one FilePlan per path in the union of the three trees, sorted
// by path, following the update table of the design (§5).
func Plan(base, ours, theirs skillfs.Tree) ([]FilePlan, error) {
	bm, om, tm := skillfs.Index(base), skillfs.Index(ours), skillfs.Index(theirs)
	set := map[string]bool{}
	for _, m := range []map[string]*skillfs.File{bm, om, tm} {
		for p := range m {
			set[p] = true
		}
	}
	paths := make([]string, 0, len(set))
	for p := range set {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	dir, err := os.MkdirTemp("", "cali-merge-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	out := make([]FilePlan, 0, len(paths))
	for _, p := range paths {
		b, o, t := bm[p], om[p], tm[p]
		switch {
		case skillfs.FileEqualPtr(o, b):
			out = append(out, take(p, TakeUpstream, t))
			continue
		case skillfs.FileEqualPtr(t, b), skillfs.FileEqualPtr(o, t):
			out = append(out, take(p, KeepLocal, o))
			continue
		}
		c := &Conflict{Ours: data(o), Base: data(b), Theirs: data(t)}
		if b == nil {
			b = &skillfs.File{Path: p} // added on both sides: merge against an empty file
		}
		if o == nil || t == nil || !skillfs.IsText(o.Data) || !skillfs.IsText(b.Data) || !skillfs.IsText(t.Data) {
			out = append(out, FilePlan{Path: p, Action: Conflicted, Conflict: c})
			continue
		}
		merged, hunks, err := merge3(dir, o.Data, b.Data, t.Data)
		if err != nil {
			return nil, fmt.Errorf("merge %s: %w", p, err)
		}
		if hunks != nil {
			c.Hunks = hunks
			out = append(out, FilePlan{Path: p, Action: Conflicted, Conflict: c})
			continue
		}
		exe := o.Exec
		if o.Exec == b.Exec {
			exe = t.Exec
		}
		out = append(out, FilePlan{Path: p, Action: Merged, Merged: EncodeLike(o.Data, merged), Exec: exe})
	}
	return out, nil
}

// EncodeLike re-encodes LF text with ref's style: CRLF if ref has more CRLF
// than lone LF line endings.
func EncodeLike(ref, lf []byte) []byte {
	crlf := bytes.Count(ref, []byte("\r\n"))
	if crlf > bytes.Count(ref, []byte("\n"))-crlf {
		return bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
	}
	return lf
}

// gitEnv drops every inherited GIT_* variable and isolates git from system,
// global and enclosing-repo config so user settings cannot change the merge.
func gitEnv(dir string) []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			env = append(env, e)
		}
	}
	return append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
}

func git(dir string, args ...string) ([]byte, int, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv(dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return stdout.Bytes(), ee.ExitCode(), fmt.Errorf("git %s: exit %d: %s", args[0], ee.ExitCode(), strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), 0, err
}

// LocalPatch returns `git diff --no-index --binary` from base to ours, with
// the trees written to dirs a/ and b/, so `git apply -p1` in a copy of base
// reproduces ours. It is empty when the trees are equal.
func LocalPatch(base, ours skillfs.Tree) ([]byte, error) {
	dir, err := os.MkdirTemp("", "cali-patch-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for name, t := range map[string]skillfs.Tree{"a": base, "b": ours} {
		if err := t.WriteUnchecked(filepath.Join(dir, name)); err != nil {
			return nil, err
		}
	}
	out, code, err := git(dir, "diff", "--no-index", "--binary", "--no-prefix", "--no-color", "--no-ext-diff", "--", "a", "b")
	if err != nil && code != 1 {
		return nil, err
	}
	return out, nil
}

// merge3 line-merges LF-normalized ours/base/theirs. It returns the merged LF
// text when clean, or the conflict hunks (non-nil) when not.
func merge3(dir string, ours, base, theirs []byte) ([]byte, []Hunk, error) {
	out, size, code, err := mergeFile(dir, ours, base, theirs)
	switch {
	case code == 0 && err == nil:
		return out, nil, nil
	case code < 1 || code > 127:
		return nil, nil, err
	}
	hunks, resolved, starts := parseConflicts(out, size)
	if len(hunks) == 0 {
		return nil, nil, fmt.Errorf("git merge-file reported %d conflicts but printed no markers", code)
	}
	if err := os.WriteFile(filepath.Join(dir, "resolved"), resolved, 0o600); err != nil {
		return nil, nil, err
	}
	if err := oursLines(dir, hunks, starts); err != nil {
		return nil, nil, err
	}
	return nil, hunks, nil
}

// mergeFile writes LF-normalized ours/base/theirs into dir and runs
// `git merge-file --diff3` with a marker size longer than any marker-like
// line prefix of the inputs, so no content line can be read as a marker.
func mergeFile(dir string, ours, base, theirs []byte) (out []byte, size, code int, err error) {
	for name, b := range map[string][]byte{"ours": ours, "base": base, "theirs": theirs} {
		if err := os.WriteFile(filepath.Join(dir, name), skillfs.NormalizeLF(b), 0o600); err != nil {
			return nil, 0, 0, err
		}
	}
	size = markerSize(ours, base, theirs)
	out, code, err = git(dir, "merge-file", "-p", "--diff3", "--marker-size="+strconv.Itoa(size),
		"-L", "ours", "-L", "base", "-L", "theirs", "ours", "base", "theirs")
	return out, size, code, err
}

// markerSize is 1 + the longest run of <, |, = or > at the start of any line
// of the sides, and at least 7 (git's default).
func markerSize(sides ...[]byte) int {
	longest := 6
	for _, b := range sides {
		for _, l := range bytes.Split(b, []byte("\n")) {
			n := 0
			for n < len(l) && bytes.IndexByte([]byte("<|=>"), l[n]) >= 0 {
				n++
			}
			longest = max(longest, n)
		}
	}
	return longest + 1
}

// parseConflicts reads diff3 output written with marker size size. resolved
// is the output with every conflict replaced by its ours side; starts[i] is
// hunk i's 1-based line there.
func parseConflicts(out []byte, size int) (hunks []Hunk, resolved []byte, starts []int) {
	open, mid := strings.Repeat("<", size)+" ours", strings.Repeat("|", size)+" base"
	sep, end := strings.Repeat("=", size), strings.Repeat(">", size)+" theirs"
	lines := strings.SplitAfter(string(out), "\n")
	state, n := 0, 0 // 0 outside, 1 ours, 2 base, 3 theirs
	var h Hunk
	var buf bytes.Buffer
	for _, l := range lines {
		s := strings.TrimSuffix(l, "\n")
		switch {
		case state == 0 && s == open:
			h, state = Hunk{}, 1
			starts = append(starts, n+1)
		case state == 1 && s == mid:
			state = 2
		case state == 2 && s == sep:
			state = 3
		case state == 3 && s == end:
			hunks, state = append(hunks, h), 0
		case state == 1:
			h.Ours = append(h.Ours, s)
			buf.WriteString(s + "\n")
			n++
		case state == 2:
			h.Base = append(h.Base, s)
		case state == 3:
			h.Theirs = append(h.Theirs, s)
		case l != "":
			buf.WriteString(l)
			n++
		}
	}
	return hunks, buf.Bytes(), starts
}

// oursLines maps each start from resolved-line to ours-line numbers. resolved
// differs from ours only by upstream's clean edits, which `git diff -U0` lists.
// shortcut: relies on diff aligning the unchanged ours sides as context; a
// repeated block could shift a reported line, which only affects display.
func oursLines(dir string, hunks []Hunk, starts []int) error {
	out, code, err := git(dir, "diff", "--no-index", "--no-color", "--no-ext-diff", "-U0", "--", "ours", "resolved")
	if err != nil && code != 1 {
		return err
	}
	type rng struct{ oldN, newStart, newN int }
	var rs []rng
	for _, l := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(l, "@@ -") {
			continue
		}
		f := strings.Fields(l) // @@ -a,b +c,d @@
		if len(f) < 3 {
			return fmt.Errorf("unexpected diff header %q", l)
		}
		_, ob := span(f[1])
		nc, nd := span(f[2])
		rs = append(rs, rng{ob, nc, nd})
	}
	for i, r := range starts {
		delta := 0
		for _, x := range rs {
			if x.newStart+max(x.newN, 1) <= r {
				delta += x.newN - x.oldN
			}
		}
		hunks[i].OursStart = r - delta
	}
	return nil
}

// span parses "-a,b" or "+c" into start and count (count defaults to 1).
func span(s string) (int, int) {
	a, b, ok := strings.Cut(s[1:], ",")
	start, _ := strconv.Atoi(a)
	if !ok {
		return start, 1
	}
	n, _ := strconv.Atoi(b)
	return start, n
}

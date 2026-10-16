package merge

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/rojcarranza/cali/internal/skillfs"
)

// SmallEdit is the largest patch, in changed lines, that counts as small (§5).
const SmallEdit = 10

// Class names the kind of a conflict (§5 "Resolving a conflict").
type Class string

// Conflict classes.
const (
	ClassDisjointHunks  Class = "disjoint-hunks"
	ClassLocalSmall     Class = "local-small"
	ClassUpstreamSmall  Class = "upstream-small"
	ClassBothLarge      Class = "both-large"
	ClassDeleteVsModify Class = "delete-vs-modify"
	ClassBinary         Class = "binary"
	ClassFrontmatter    Class = "frontmatter"
)

// Recommendation is Cali's suggested resolution of one conflicting file.
type Recommendation struct {
	Class  Class
	Choice string // "theirs", "ours" or "merge"
	Reason string // one line, never file content
	Safe   bool   // eligible for --accept-recommended
}

// Classify recommends a resolution for the conflict at path (relative to the
// skill root). localLines and upstreamLines are the changed-line counts of
// base to ours and base to theirs (see ChangedLines). A nil side is deleted.
func Classify(path string, c Conflict, localLines, upstreamLines int) Recommendation {
	counts := fmt.Sprintf("local edit is %d lines, upstream edit is %d lines", localLines, upstreamLines)
	rec := func(cl Class, choice string, safe bool, why string) Recommendation {
		return Recommendation{Class: cl, Choice: choice, Reason: counts + "; " + why, Safe: safe}
	}
	switch {
	case binary(c.Ours) || binary(c.Base) || binary(c.Theirs):
		return rec(ClassBinary, "ours", false, "binary file changed on both sides and Cali cannot show a diff, so keep mine until reviewed")
	case c.Ours == nil:
		return rec(ClassDeleteVsModify, "theirs", true, "the local side deleted the file and upstream changed it, so take upstream")
	case c.Theirs == nil:
		return rec(ClassDeleteVsModify, "ours", true, "upstream deleted the file and the local side changed it, so keep mine")
	case path == "SKILL.md" && inFrontmatter(c):
		return rec(ClassFrontmatter, "ours", false, "the conflict is inside the SKILL.md frontmatter and name and description drive loading, so keep mine and compare both versions of the keys")
	case len(c.Hunks) > 0 && allDisjoint(c.Hunks):
		if _, ok := ResolveDisjoint(c); ok {
			return rec(ClassDisjointHunks, "merge", true, "the changes do not overlap and only touch adjacent lines, so merge both")
		}
	}
	local, upstream := localLines <= SmallEdit, upstreamLines <= SmallEdit
	switch {
	case local && (!upstream || localLines <= upstreamLines):
		return rec(ClassLocalSmall, "theirs", true, "upstream rewrote more of this file, so take upstream and re-apply the saved local patch by hand")
	case upstream:
		return rec(ClassUpstreamSmall, "ours", true, "the local edit is the larger one, so keep mine and copy the upstream patch over")
	}
	return rec(ClassBothLarge, "ours", false, "both sides changed more than "+strconv.Itoa(SmallEdit)+" lines, so keep mine until reviewed")
}

func binary(b []byte) bool { return b != nil && !skillfs.IsText(b) }

// inFrontmatter reports whether a hunk starts within ours' frontmatter, or
// the frontmatter blocks of base, ours and theirs are not all identical.
func inFrontmatter(c Conflict) bool {
	ob, end, ok := frontmatter(c.Ours)
	bb, _, bok := frontmatter(c.Base)
	tb, _, tok := frontmatter(c.Theirs)
	if ok != bok || ok != tok || ob != bb || ob != tb {
		return true
	}
	for _, h := range c.Hunks {
		if ok && h.OursStart <= end {
			return true
		}
	}
	return false
}

// frontmatter returns b's frontmatter block (LF) from the first-line ---
// through the closing --- and that line's number; an unclosed block runs to EOF.
func frontmatter(b []byte) (string, int, bool) {
	lines := strings.Split(string(skillfs.NormalizeLF(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))), "\n")
	if lines[0] != "---" {
		return "", 0, false
	}
	end := len(lines)
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i + 1
			break
		}
	}
	return strings.Join(lines[:end], "\n"), end, true
}

// edit replaces base lines [s, e) with repl; s == e is an insertion.
type edit struct {
	s, e int
	repl []string
}

// edits returns the line edits that turn a into b, from a longest common subsequence.
func edits(a, b []string) []edit {
	n, m := len(a), len(b)
	l := make([][]int, n+1)
	for i := range l {
		l[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				l[i][j] = l[i+1][j+1] + 1
			} else {
				l[i][j] = max(l[i+1][j], l[i][j+1])
			}
		}
	}
	var out []edit
	inEdit := false
	for i, j := 0, 0; i < n || j < m; {
		if i < n && j < m && a[i] == b[j] {
			inEdit = false
			i, j = i+1, j+1
			continue
		}
		if !inEdit {
			out = append(out, edit{s: i, e: i})
			inEdit = true
		}
		cur := &out[len(out)-1]
		if j < m && (i == n || l[i][j+1] >= l[i+1][j]) {
			cur.repl = append(cur.repl, b[j])
			j++
		} else {
			i++
			cur.e = i
		}
	}
	return out
}

// mergeHunk applies both sides' edits to the hunk's base, or reports false if
// any edit of ours overlaps one of theirs or both start at the same base line.
func mergeHunk(h Hunk) ([]string, bool) {
	eo, et := edits(h.Base, h.Ours), edits(h.Base, h.Theirs)
	for _, a := range eo {
		for _, b := range et {
			if a.s == b.s || (a.s < b.e && b.s < a.e) {
				return nil, false
			}
		}
	}
	all := append(eo, et...)
	sort.Slice(all, func(i, j int) bool { return all[i].s < all[j].s })
	out, i := []string{}, 0
	for _, e := range all {
		out = append(append(out, h.Base[i:e.s]...), e.repl...)
		i = e.e
	}
	return append(out, h.Base[i:]...), true
}

func allDisjoint(hs []Hunk) bool {
	for _, h := range hs {
		if _, ok := mergeHunk(h); !ok {
			return false
		}
	}
	return true
}

// ResolveDisjoint re-runs the line merge of c and applies both sides' changes
// inside every conflicting hunk. It returns the whole merged file as LF text,
// or false if a side is deleted or binary, or any hunk is not disjoint. The
// caller must re-encode the result with ours' line-ending style.
func ResolveDisjoint(c Conflict) ([]byte, bool) {
	if c.Ours == nil || c.Theirs == nil || binary(c.Base) || binary(c.Ours) || binary(c.Theirs) {
		return nil, false
	}
	dir, err := os.MkdirTemp("", "cali-resolve-*")
	if err != nil {
		return nil, false
	}
	defer func() { _ = os.RemoveAll(dir) }()
	out, size, code, err := mergeFile(dir, c.Ours, c.Base, c.Theirs)
	switch {
	case code == 0 && err == nil:
		return out, true
	case code < 1 || code > 127:
		return nil, false
	}
	hunks, resolved, starts := parseConflicts(out, size)
	if len(hunks) == 0 {
		return nil, false
	}
	// resolved holds each conflict as its ours side, starting at starts[i].
	lines := strings.SplitAfter(string(resolved), "\n")
	var buf strings.Builder
	pos := 0
	for i, h := range hunks {
		merged, ok := mergeHunk(h)
		if !ok {
			return nil, false
		}
		buf.WriteString(strings.Join(lines[pos:starts[i]-1], ""))
		for _, l := range merged {
			buf.WriteString(l + "\n")
		}
		pos = starts[i] - 1 + len(h.Ours)
	}
	buf.WriteString(strings.Join(lines[pos:], ""))
	return []byte(buf.String()), true
}

// ChangedLines counts added plus removed lines from base to side, after LF
// normalization, using `git diff --no-index --numstat`. A nil side counts as
// an empty file. Binary input is refused.
func ChangedLines(base, side []byte) (int, error) {
	if !skillfs.IsText(base) || !skillfs.IsText(side) {
		return 0, errors.New("binary file: cannot count changed lines")
	}
	dir, err := os.MkdirTemp("", "cali-count-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	for name, b := range map[string][]byte{"base": base, "side": side} {
		if err := os.WriteFile(filepath.Join(dir, name), skillfs.NormalizeLF(b), 0o600); err != nil {
			return 0, err
		}
	}
	out, code, err := git(dir, "diff", "--no-index", "--no-color", "--no-ext-diff", "--numstat", "--", "base", "side")
	if err != nil && code != 1 { // exit 1 means the files differ
		return 0, err
	}
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l == "" {
			continue
		}
		f := strings.Fields(l) // added removed path; "-" for binary
		if len(f) < 3 {
			return 0, fmt.Errorf("git diff --numstat: unexpected line %q", l)
		}
		a, errA := strconv.Atoi(f[0])
		r, errR := strconv.Atoi(f[1])
		if errA != nil || errR != nil {
			return 0, errors.New("git diff --numstat: binary file or bad count")
		}
		n += a + r
	}
	return n, nil
}

// PreResolve settles a conflict where one side differs from base only in line
// endings or trailing spaces and tabs: it returns the other side. A nil result
// with ok means the file is deleted. Binary input and deletions never pre-resolve.
func PreResolve(base, ours, theirs []byte) ([]byte, bool) {
	if binary(base) || binary(ours) || binary(theirs) {
		return nil, false
	}
	switch {
	case ours != nil && sameLines(base, ours):
		return theirs, true
	case theirs != nil && sameLines(base, theirs):
		return ours, true
	}
	return nil, false
}

// sameLines reports whether a and b have the same LF lines once trailing
// spaces and tabs are trimmed; indentation and line breaks still count.
func sameLines(a, b []byte) bool {
	la := strings.Split(string(skillfs.NormalizeLF(a)), "\n")
	lb := strings.Split(string(skillfs.NormalizeLF(b)), "\n")
	if len(la) != len(lb) {
		return false
	}
	for i := range la {
		if strings.TrimRight(la[i], " \t") != strings.TrimRight(lb[i], " \t") {
			return false
		}
	}
	return true
}

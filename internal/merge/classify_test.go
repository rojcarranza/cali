package merge

import (
	"strings"
	"testing"
)

// overlap is a text conflict with one overlapping hunk outside any frontmatter.
var overlap = Conflict{
	Ours: []byte("a\nOURS\nc\n"), Base: []byte("a\nb\nc\n"), Theirs: []byte("a\nTHEIRS\nc\n"),
	Hunks: []Hunk{{OursStart: 2, Ours: []string{"OURS"}, Base: []string{"b"}, Theirs: []string{"THEIRS"}}},
}

func conflictOf(t *testing.T, path, base, ours, theirs string) Conflict {
	t.Helper()
	ps, err := Plan(tr(f(path, base)), tr(f(path, ours)), tr(f(path, theirs)))
	if err != nil {
		t.Fatal(err)
	}
	p := byPath(t, ps, path)
	if p.Action != Conflicted || p.Conflict == nil {
		t.Fatalf("setup: %s is %s, want a conflict", path, p.Action)
	}
	return *p.Conflict
}

func check(t *testing.T, r Recommendation, class Class, choice string, safe bool, counts ...string) {
	t.Helper()
	if r.Class != class || r.Choice != choice || r.Safe != safe {
		t.Errorf("got %s/%s/safe=%v, want %s/%s/safe=%v (reason %q)", r.Class, r.Choice, r.Safe, class, choice, safe, r.Reason)
	}
	for _, c := range counts {
		if !strings.Contains(r.Reason, c) {
			t.Errorf("reason %q does not contain %q", r.Reason, c)
		}
	}
	if strings.Contains(r.Reason, "\n") {
		t.Errorf("reason %q is not one line", r.Reason)
	}
}

func TestClassifyLocalSmall(t *testing.T) {
	check(t, Classify("ref.md", overlap, 3, 48), ClassLocalSmall, "theirs", true, "3 lines", "48 lines")
}

func TestClassifyUpstreamSmall(t *testing.T) {
	check(t, Classify("ref.md", overlap, 48, 3), ClassUpstreamSmall, "ours", true, "48 lines", "3 lines")
}

func TestBothLargeNotSafe(t *testing.T) {
	check(t, Classify("ref.md", overlap, 20, 30), ClassBothLarge, "ours", false, "20 lines", "30 lines")
}

func TestClassifyBothSmallSmallerSideDecides(t *testing.T) {
	check(t, Classify("ref.md", overlap, 2, 5), ClassLocalSmall, "theirs", true, "2 lines", "5 lines")
	check(t, Classify("ref.md", overlap, 4, 4), ClassLocalSmall, "theirs", true, "4 lines")
	check(t, Classify("ref.md", overlap, 6, 1), ClassUpstreamSmall, "ours", true, "6 lines", "1 lines")
}

func TestThresholdBoundary(t *testing.T) {
	check(t, Classify("ref.md", overlap, SmallEdit, SmallEdit+1), ClassLocalSmall, "theirs", true, "10 lines", "11 lines")
	check(t, Classify("ref.md", overlap, SmallEdit+1, SmallEdit), ClassUpstreamSmall, "ours", true, "11 lines", "10 lines")
	check(t, Classify("ref.md", overlap, SmallEdit+1, SmallEdit+1), ClassBothLarge, "ours", false, "11 lines")
}

func TestBinaryNotSafe(t *testing.T) {
	c := Conflict{Ours: []byte("o\x00"), Base: []byte("b\x00"), Theirs: []byte("t\x00")}
	check(t, Classify("img.bin", c, 1, 1), ClassBinary, "ours", false, "1 lines")
}

func TestClassifyBinarySkillMDIsBinary(t *testing.T) {
	c := Conflict{Ours: []byte("---\x00"), Base: []byte("---\nname: x\n---\n"), Theirs: []byte("---\nname: y\n---\n"),
		Hunks: []Hunk{{OursStart: 1}}}
	check(t, Classify("SKILL.md", c, 3, 2), ClassBinary, "ours", false)
}

func TestClassifyDeleteVsModify(t *testing.T) {
	localDeleted := Conflict{Ours: nil, Base: []byte("a\n"), Theirs: []byte("A\n")}
	check(t, Classify("ref.md", localDeleted, 1, 2), ClassDeleteVsModify, "theirs", true, "1 lines", "2 lines")
	upstreamDeleted := Conflict{Ours: []byte("A\n"), Base: []byte("a\n"), Theirs: nil}
	check(t, Classify("ref.md", upstreamDeleted, 2, 1), ClassDeleteVsModify, "ours", true, "2 lines", "1 lines")
}

func TestClassifyDeleteVsModifyFromPlan(t *testing.T) {
	ps, err := Plan(tr(f("a.md", "a\n")), tr(), tr(f("a.md", "A\n")))
	if err != nil {
		t.Fatal(err)
	}
	check(t, Classify("a.md", *byPath(t, ps, "a.md").Conflict, 1, 2), ClassDeleteVsModify, "theirs", true)
}

const skillBase = "---\nname: x\ndescription: d\n---\n# body\n"

func TestFrontmatterNeverRecommendsTheirs(t *testing.T) {
	c := conflictOf(t, "SKILL.md", skillBase,
		"---\nname: x\ndescription: mine\n---\n# body\n",
		"---\nname: x\ndescription: theirs\n---\n# body\n")
	for _, n := range [][2]int{{1, 48}, {3, 3}, {48, 48}, {48, 1}} {
		r := Classify("SKILL.md", c, n[0], n[1])
		check(t, r, ClassFrontmatter, "ours", false)
		if r.Choice == "theirs" || !strings.Contains(r.Reason, "name") || !strings.Contains(r.Reason, "description") {
			t.Errorf("counts %v: got %+v", n, r)
		}
	}
}

func TestClassifyFrontmatterOnlyAtSkillRoot(t *testing.T) {
	c := conflictOf(t, "sub/SKILL.md", skillBase,
		"---\nname: x\ndescription: mine\n---\n# body\n",
		"---\nname: x\ndescription: theirs\n---\n# body\n")
	check(t, Classify("sub/SKILL.md", c, 1, 48), ClassLocalSmall, "theirs", true)
}

func TestClassifySkillMDBodyConflictIsNotFrontmatter(t *testing.T) {
	c := conflictOf(t, "SKILL.md", skillBase,
		"---\nname: x\ndescription: d\n---\n# mine\n",
		"---\nname: x\ndescription: d\n---\n# theirs\n")
	check(t, Classify("SKILL.md", c, 1, 48), ClassLocalSmall, "theirs", true, "1 lines", "48 lines")
}

func TestClassifyDisjointHunks(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\nb\nc\nd\n", "a\nB\nc\nd\n", "a\nb\nC\nd\n")
	check(t, Classify("ref.md", c, 2, 2), ClassDisjointHunks, "merge", true, "2 lines")
}

func TestClassifyOverlapIsNotDisjoint(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\nb\nc\n", "a\nOURS\nc\n", "a\nTHEIRS\nc\n")
	check(t, Classify("ref.md", c, 2, 2), ClassLocalSmall, "theirs", true)
}

func TestResolveDisjointMergesAdjacentEdits(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\nb\nc\nd\ne\nf\ng\nh\n",
		"a\nB\nc\nd\ne\nf\ng\nh\n",        // ours edits b
		"a\nb\nC\nd\ne\nf\ng\nH\nextra\n") // theirs edits c (adjacent) and h (clean)
	got, ok := ResolveDisjoint(c)
	if want := "a\nB\nC\nd\ne\nf\ng\nH\nextra\n"; !ok || string(got) != want {
		t.Errorf("got %q %v, want %q true", got, ok, want)
	}
}

func TestResolveDisjointCRLFReturnsLF(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\r\nb\r\nc\r\n", "A\r\nb\r\nc\r\n", "a\r\nB\r\nc\r\n")
	got, ok := ResolveDisjoint(c)
	if want := "A\nB\nc\n"; !ok || string(got) != want {
		t.Errorf("got %q %v, want %q true", got, ok, want)
	}
}

func TestResolveDisjointRefusesOverlap(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\nb\nc\n", "a\nOURS\nc\n", "a\nTHEIRS\nc\n")
	if got, ok := ResolveDisjoint(c); ok || got != nil {
		t.Errorf("got %q %v, want nil false", got, ok)
	}
}

func TestResolveDisjointRefusesSameInsertionPoint(t *testing.T) {
	c := Conflict{Ours: []byte("a\nX\nb\n"), Base: []byte("a\nb\n"), Theirs: []byte("a\nY\nb\n"),
		Hunks: []Hunk{{OursStart: 2, Ours: []string{"X"}, Theirs: []string{"Y"}}}}
	if _, ok := ResolveDisjoint(c); ok {
		t.Error("two insertions at one point must not merge")
	}
}

func TestPreResolveWhitespaceOnly(t *testing.T) {
	base := []byte("a b\nc\n")
	for _, tc := range []struct {
		name, ours, theirs, want string
		ok                       bool
	}{
		{"ours CRLF only", "a b\r\nc\r\n", "X\n", "X\n", true},
		{"theirs trailing space only", "Y\n", "a b \t\nc  \n", "Y\n", true},
		{"both real", "Y\n", "X\n", "", false},
	} {
		got, ok := PreResolve(base, []byte(tc.ours), []byte(tc.theirs))
		if ok != tc.ok || string(got) != tc.want {
			t.Errorf("%s: got %q %v, want %q %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestPreResolveDeletionIsNotWhitespace(t *testing.T) {
	if got, ok := PreResolve([]byte("\n"), nil, []byte("X\n")); ok {
		t.Errorf("deleting the file is a real change, got %q", got)
	}
}

func TestChangedLines(t *testing.T) {
	n, err := ChangedLines([]byte("a\nb\nc\n"), []byte("a\r\nB\r\nc\r\nd\r\n"))
	if err != nil || n != 3 { // b removed, B and d added; CRLF alone is no change
		t.Errorf("got %d %v, want 3", n, err)
	}
	if n, err := ChangedLines([]byte("a\n"), []byte("a\r\n")); err != nil || n != 0 {
		t.Errorf("same text: got %d %v, want 0", n, err)
	}
	if n, err := ChangedLines([]byte("a\nb\n"), nil); err != nil || n != 2 {
		t.Errorf("deleted side: got %d %v, want 2", n, err)
	}
}

func TestChangedLinesRefusesBinary(t *testing.T) {
	if _, err := ChangedLines([]byte("a\n"), []byte("a\x00\n")); err == nil {
		t.Error("want an error for binary input")
	}
}

func TestResolveDisjointMarkerLineInBase(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\n=======\n", "a\n", "=======\nc\n")
	got, ok := ResolveDisjoint(c)
	if !ok || string(got) != "c\n" {
		t.Errorf("got %q %v, want %q true", got, ok, "c\n")
	}
}

func TestResolveDisjointMarkerLineIsConflict(t *testing.T) {
	c := conflictOf(t, "ref.md", "a\nx\n=======\n", "x\na\nx\nx\n=======\n", "a\nb\n")
	if got, ok := ResolveDisjoint(c); ok {
		t.Errorf("got clean merge %q, want a conflict", got)
	}
}

func TestPreResolveKeepsRealWhitespaceChanges(t *testing.T) {
	base := []byte("def f():\n    return 1\n\nend\n")
	for name, ours := range map[string]string{
		"indent":       "def f():\n  return 1\n\nend\n",
		"join":         "def f(): return 1\n\nend\n",
		"split":        "def f():\n    return\n    1\n\nend\n",
		"blank insert": "def f():\n\n    return 1\n\nend\n",
	} {
		if got, ok := PreResolve(base, []byte(ours), []byte("X\n")); ok {
			t.Errorf("%s: pre-resolved to %q, want no pre-resolution", name, got)
		}
	}
}

func TestClassifyFrontmatterRemovedLocally(t *testing.T) {
	c := conflictOf(t, "SKILL.md", skillBase, "# mine\n", "---\nname: x\ndescription: d\n---\n# theirs\n")
	check(t, Classify("SKILL.md", c, 4, 1), ClassFrontmatter, "ours", false)
}

func TestClassifyFrontmatterUpstreamOnlyChange(t *testing.T) {
	c := conflictOf(t, "SKILL.md", skillBase,
		"---\nname: x\ndescription: d\n---\n# mine\n",
		"---\nname: x\ndescription: new\n---\n# theirs\n")
	for _, h := range c.Hunks {
		if h.OursStart <= 4 {
			t.Fatalf("setup: hunk %+v overlaps the frontmatter", h)
		}
	}
	check(t, Classify("SKILL.md", c, 1, 48), ClassFrontmatter, "ours", false)
}

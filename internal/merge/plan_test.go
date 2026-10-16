package merge

import (
	"bytes"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/skillfs"
)

func f(path, data string) skillfs.File { return skillfs.File{Path: path, Data: []byte(data)} }

func tr(files ...skillfs.File) skillfs.Tree { return skillfs.Tree(files) }

type fixture struct{ base, ours, theirs skillfs.Tree }

const base5 = "a\nb\nc\nd\ne\n"

// fixtures is shared so TestPlanNeverEmitsMarkers checks every plan.
var fixtures = map[string]fixture{
	"ours-unchanged": {
		base:   tr(f("SKILL.md", base5), f("gone.md", "x\n"), f("same.md", "s\n")),
		ours:   tr(f("SKILL.md", base5), f("gone.md", "x\n"), f("same.md", "s\n")),
		theirs: tr(f("SKILL.md", "a\nB\nc\nd\ne\n"), f("new.md", "n\n"), f("same.md", "s\n")),
	},
	"theirs-unchanged": {
		base:   tr(f("SKILL.md", base5), f("gone.md", "x\n")),
		ours:   tr(f("SKILL.md", "A\nb\nc\nd\ne\n"), f("local.md", "l\n")),
		theirs: tr(f("SKILL.md", base5), f("gone.md", "x\n")),
	},
	"delete-vs-unchanged": {
		base:   tr(f("a.md", "a\n"), f("b.md", "b\n")),
		ours:   tr(f("b.md", "b\n")),
		theirs: tr(f("a.md", "a\n")),
	},
	"same-change": {
		base:   tr(f("SKILL.md", base5)),
		ours:   tr(f("SKILL.md", "a\nX\nc\nd\ne\n")),
		theirs: tr(f("SKILL.md", "a\nX\nc\nd\ne\n")),
	},
	"disjoint": {
		base:   tr(f("SKILL.md", base5)),
		ours:   tr(f("SKILL.md", "A\nb\nc\nd\ne\n")),
		theirs: tr(f("SKILL.md", "a\nb\nc\nd\nE\n")),
	},
	"overlap": {
		base:   tr(f("SKILL.md", base5)),
		ours:   tr(f("SKILL.md", "a\nb\nOURS\nd\ne\n")),
		theirs: tr(f("SKILL.md", "a\nb\nTHEIRS\nd\ne\n")),
	},
	"overlap-shifted": { // theirs inserts lines above the conflict; OursStart must stay in ours coordinates
		base:   tr(f("SKILL.md", "1\n2\n3\n4\n5\n6\n7\n8\n")),
		ours:   tr(f("SKILL.md", "1\n2\n3\n4\n5\n6\nO7\n8\n")),
		theirs: tr(f("SKILL.md", "1\nt1\nt2\nt3\n2\n3\n4\n5\n6\nT7\n8\n")),
	},
	"binary": {
		base:   tr(f("img.bin", "\x00base")),
		ours:   tr(f("img.bin", "\x00ours")),
		theirs: tr(f("img.bin", "\x00theirs")),
	},
	"delete-vs-modify": {
		base:   tr(f("a.md", "a\n"), f("b.md", "b\n")),
		ours:   tr(f("b.md", "B ours\n")),
		theirs: tr(f("a.md", "A theirs\n")),
	},
	"add-add": {
		base:   tr(),
		ours:   tr(f("new.md", "ours\n")),
		theirs: tr(f("new.md", "theirs\n")),
	},
	"crlf": {
		base:   tr(f("SKILL.md", base5)),
		ours:   tr(f("SKILL.md", "A\r\nb\r\nc\r\nd\r\ne\r\n")),
		theirs: tr(f("SKILL.md", "a\nb\nc\nd\nE\n")),
	},
	"legit-markers": {
		base:   tr(f("SKILL.md", "a\n<<<<<<< x\nb\n=======\nc\n>>>>>>> y\nd\n|||||||\ne\nf\n")),
		ours:   tr(f("SKILL.md", "A\n<<<<<<< x\nb\n=======\nc\n>>>>>>> y\nd\n|||||||\ne\nf\n")),
		theirs: tr(f("SKILL.md", "a\n<<<<<<< x\nb\n=======\nc\n>>>>>>> y\nd\n|||||||\ne\nF\n")),
	},
	"exec-only": {
		base:   tr(f("run.sh", "echo\n")),
		ours:   tr(skillfs.File{Path: "run.sh", Exec: true, Data: []byte("echo\n")}),
		theirs: tr(f("run.sh", "echo\n")),
	},
}

func plan(t *testing.T, name string) []FilePlan {
	t.Helper()
	fx := fixtures[name]
	p, err := Plan(fx.base, fx.ours, fx.theirs)
	if err != nil {
		t.Fatalf("Plan(%s): %v", name, err)
	}
	return p
}

func byPath(t *testing.T, ps []FilePlan, path string) FilePlan {
	t.Helper()
	for _, p := range ps {
		if p.Path == path {
			return p
		}
	}
	t.Fatalf("no plan for %s in %+v", path, ps)
	return FilePlan{}
}

func TestPlanOursUnchangedTakesTheirs(t *testing.T) {
	ps := plan(t, "ours-unchanged")
	var paths []string
	for _, p := range ps {
		paths = append(paths, p.Path)
	}
	if want := []string{"SKILL.md", "gone.md", "new.md", "same.md"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if p := byPath(t, ps, "SKILL.md"); p.Action != TakeUpstream || string(p.Merged) != "a\nB\nc\nd\ne\n" {
		t.Errorf("SKILL.md = %+v", p)
	}
	if p := byPath(t, ps, "gone.md"); p.Action != TakeUpstream || !p.Delete {
		t.Errorf("gone.md = %+v, want TakeUpstream Delete", p)
	}
	if p := byPath(t, ps, "new.md"); p.Action != TakeUpstream || string(p.Merged) != "n\n" {
		t.Errorf("new.md = %+v", p)
	}
	if p := byPath(t, ps, "same.md"); p.Action != TakeUpstream || string(p.Merged) != "s\n" {
		t.Errorf("same.md = %+v", p)
	}
}

func TestPlanTheirsUnchangedKeepsOurs(t *testing.T) {
	ps := plan(t, "theirs-unchanged")
	if p := byPath(t, ps, "SKILL.md"); p.Action != KeepLocal || string(p.Merged) != "A\nb\nc\nd\ne\n" {
		t.Errorf("SKILL.md = %+v", p)
	}
	if p := byPath(t, ps, "gone.md"); p.Action != KeepLocal || !p.Delete {
		t.Errorf("gone.md = %+v, want KeepLocal Delete", p)
	}
	if p := byPath(t, ps, "local.md"); p.Action != KeepLocal || string(p.Merged) != "l\n" {
		t.Errorf("local.md = %+v", p)
	}
}

func TestPlanDeleteVsUnchanged(t *testing.T) {
	ps := plan(t, "delete-vs-unchanged")
	if p := byPath(t, ps, "a.md"); p.Action != KeepLocal || !p.Delete {
		t.Errorf("a.md (ours deleted, theirs unchanged) = %+v, want KeepLocal Delete", p)
	}
	if p := byPath(t, ps, "b.md"); p.Action != TakeUpstream || !p.Delete {
		t.Errorf("b.md (theirs deleted, ours unchanged) = %+v, want TakeUpstream Delete", p)
	}
}

func TestPlanSameChangeKeeps(t *testing.T) {
	p := byPath(t, plan(t, "same-change"), "SKILL.md")
	if p.Action != KeepLocal || string(p.Merged) != "a\nX\nc\nd\ne\n" {
		t.Errorf("got %+v", p)
	}
}

func TestPlanDisjointEditsMerge(t *testing.T) {
	p := byPath(t, plan(t, "disjoint"), "SKILL.md")
	if p.Action != Merged || string(p.Merged) != "A\nb\nc\nd\nE\n" || p.Conflict != nil {
		t.Errorf("got %+v (merged %q)", p, p.Merged)
	}
}

func TestPlanOverlapIsConflict(t *testing.T) {
	p := byPath(t, plan(t, "overlap"), "SKILL.md")
	if p.Action != Conflicted || p.Merged != nil || p.Conflict == nil {
		t.Fatalf("got %+v", p)
	}
	want := []Hunk{{OursStart: 3, Ours: []string{"OURS"}, Base: []string{"c"}, Theirs: []string{"THEIRS"}}}
	if !reflect.DeepEqual(p.Conflict.Hunks, want) {
		t.Errorf("hunks = %+v, want %+v", p.Conflict.Hunks, want)
	}
	if string(p.Conflict.Ours) != "a\nb\nOURS\nd\ne\n" || string(p.Conflict.Base) != base5 || string(p.Conflict.Theirs) != "a\nb\nTHEIRS\nd\ne\n" {
		t.Errorf("conflict sides = %q %q %q", p.Conflict.Ours, p.Conflict.Base, p.Conflict.Theirs)
	}
}

func TestPlanHunkOursStartInOursLines(t *testing.T) {
	p := byPath(t, plan(t, "overlap-shifted"), "SKILL.md")
	if p.Action != Conflicted || p.Conflict == nil {
		t.Fatalf("got %+v", p)
	}
	want := []Hunk{{OursStart: 7, Ours: []string{"O7"}, Base: []string{"7"}, Theirs: []string{"T7"}}}
	if !reflect.DeepEqual(p.Conflict.Hunks, want) {
		t.Errorf("hunks = %+v, want %+v", p.Conflict.Hunks, want)
	}
}

func TestPlanBinaryBothChangedConflict(t *testing.T) {
	p := byPath(t, plan(t, "binary"), "img.bin")
	if p.Action != Conflicted || p.Conflict == nil || len(p.Conflict.Hunks) != 0 || p.Merged != nil {
		t.Errorf("got %+v", p)
	}
}

func TestPlanDeleteVsModifyConflict(t *testing.T) {
	ps := plan(t, "delete-vs-modify")
	for _, path := range []string{"a.md", "b.md"} {
		if p := byPath(t, ps, path); p.Action != Conflicted || p.Conflict == nil || p.Merged != nil {
			t.Errorf("%s = %+v, want Conflict", path, p)
		}
	}
}

func TestPlanAddAddConflict(t *testing.T) {
	p := byPath(t, plan(t, "add-add"), "new.md")
	if p.Action != Conflicted || p.Conflict == nil {
		t.Fatalf("got %+v", p)
	}
	want := []Hunk{{OursStart: 1, Ours: []string{"ours"}, Base: nil, Theirs: []string{"theirs"}}}
	if !reflect.DeepEqual(p.Conflict.Hunks, want) {
		t.Errorf("hunks = %#v, want %#v", p.Conflict.Hunks, want)
	}
}

func TestPlanPreservesCRLFStyle(t *testing.T) {
	p := byPath(t, plan(t, "crlf"), "SKILL.md")
	if p.Action != Merged || string(p.Merged) != "A\r\nb\r\nc\r\nd\r\nE\r\n" {
		t.Errorf("got %+v (merged %q)", p, p.Merged)
	}
}

func TestPlanLegitMarkersMergeCleanly(t *testing.T) {
	p := byPath(t, plan(t, "legit-markers"), "SKILL.md")
	want := "A\n<<<<<<< x\nb\n=======\nc\n>>>>>>> y\nd\n|||||||\ne\nF\n"
	if p.Action != Merged || string(p.Merged) != want {
		t.Errorf("got %+v (merged %q)", p, p.Merged)
	}
}

func TestPlanExecBitOnlyChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("exec bits are not compared on windows")
	}
	p := byPath(t, plan(t, "exec-only"), "run.sh")
	if p.Action != KeepLocal || !p.Exec || string(p.Merged) != "echo\n" {
		t.Errorf("got %+v", p)
	}
}

var markers = []string{"<<<<<<<", "|||||||", "=======", ">>>>>>>"}

// markerLines returns the marker-prefixed lines of b.
func markerLines(b []byte) map[string]bool {
	out := map[string]bool{}
	for _, l := range strings.Split(string(bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))), "\n") {
		for _, m := range markers {
			if strings.HasPrefix(l, m) {
				out[l] = true
			}
		}
	}
	return out
}

func TestPlanNeverEmitsMarkers(t *testing.T) {
	for name, fx := range fixtures {
		ps, err := Plan(fx.base, fx.ours, fx.theirs)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		legit := map[string]bool{}
		for _, side := range []skillfs.Tree{fx.ours, fx.theirs} {
			for _, file := range side {
				for l := range markerLines(file.Data) {
					legit[l] = true
				}
			}
		}
		for _, p := range ps {
			for l := range markerLines(p.Merged) {
				if !legit[l] {
					t.Errorf("%s/%s: Merged contains marker line %q", name, p.Path, l)
				}
			}
		}
	}
}

func TestPlanHunkWithMarkerContentLine(t *testing.T) {
	ps, err := Plan(tr(f("SKILL.md", "a\n=======\nc\n")), tr(f("SKILL.md", "a\nO\nc\n")), tr(f("SKILL.md", "a\nT\nc\n")))
	if err != nil {
		t.Fatal(err)
	}
	p := byPath(t, ps, "SKILL.md")
	want := []Hunk{{OursStart: 2, Ours: []string{"O"}, Base: []string{"======="}, Theirs: []string{"T"}}}
	if p.Action != Conflicted || p.Conflict == nil || !reflect.DeepEqual(p.Conflict.Hunks, want) {
		t.Fatalf("got %+v, want hunks %+v", p, want)
	}
}

func TestMarkerSize(t *testing.T) {
	if n := markerSize([]byte("a\n")); n != 7 {
		t.Errorf("plain text: got %d, want 7", n)
	}
	if n := markerSize([]byte("a\n"), []byte("x\n<<<|=>>>>> y\n"), nil); n != 11 { // 10-char prefix
		t.Errorf("got %d, want 11", n)
	}
}

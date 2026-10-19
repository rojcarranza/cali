package render

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestModeFor(t *testing.T) {
	cases := []struct {
		tty, json bool
		want      Mode
	}{
		{false, false, Plain},
		{true, false, Styled},
		{true, true, JSON},
		{false, true, JSON},
	}
	for _, c := range cases {
		if got := ModeFor(c.tty, c.json); got != c.want {
			t.Errorf("ModeFor(%v,%v)=%v want %v", c.tty, c.json, got, c.want)
		}
	}
}

func TestPlainTableHasNoANSI(t *testing.T) {
	var b bytes.Buffer
	Table{Header: []string{"NAME", "REF"}, Rows: [][]string{{"a\x1b[31mred", "v1\x00\u009b"}}}.Render(&b, Plain)
	out := b.String()
	if strings.ContainsRune(out, 0x1b) || strings.ContainsAny(out, "\x00\u009b") {
		t.Fatalf("control chars in output: %q", out)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "a[31mred") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestStyledTableStripsControl(t *testing.T) {
	var b bytes.Buffer
	Table{Header: []string{"NAME"}, Rows: [][]string{{"x\x1b]0;evil\x07y"}}}.Render(&b, Styled)
	if strings.Contains(b.String(), "evil\x07") || strings.ContainsRune(b.String(), 0x07) {
		t.Fatalf("control char survived: %q", b.String())
	}
}

func TestSanitize(t *testing.T) {
	if got := sanitize("a\tb\n\x1b\x7f\u0085c"); got != "a b c" {
		t.Fatalf("got %q", got)
	}
}

func TestJSONHasSchemaVersion(t *testing.T) {
	var b bytes.Buffer
	if err := WriteJSON(&b, map[string]any{"skills": []string{"x"}, "a": 1}); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"schema_version\": 1,\n  \"a\": 1,\n  \"skills\": [\n    \"x\"\n  ]\n}\n"
	if b.String() != want {
		t.Fatalf("got %q want %q", b.String(), want)
	}
	b.Reset()
	type s struct {
		Z int `json:"z"`
		A int `json:"a"`
	}
	if err := WriteJSON(&b, s{1, 2}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "{\n  \"schema_version\": 1,\n  \"z\": 1,\n  \"a\": 2") {
		t.Fatalf("struct order: %q", b.String())
	}
}

func TestWriteJSONRejectsSchemaVersion(t *testing.T) {
	type st struct {
		V int `json:"schema_version"`
	}
	for _, v := range []any{st{7}, map[string]any{"schema_version": 7}} {
		var b bytes.Buffer
		if err := WriteJSON(&b, v); err == nil || b.Len() != 0 {
			t.Fatalf("%T: want error and no output, got %v %q", v, err, b.String())
		}
	}
}

func TestWriteJSONNoHTMLEscape(t *testing.T) {
	var b bytes.Buffer
	if err := WriteJSON(&b, map[string]string{"u": "a<b>&c"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"a<b>&c"`) {
		t.Fatalf("escaped: %q", b.String())
	}
}

func TestSanitizeDropsFormatRunes(t *testing.T) {
	if got := Sanitize("a\u202eb\u200bc\u2066d\ufeff"); got != "abcd" {
		t.Fatalf("got %q", got)
	}
}

func TestStyledTableFitsTerminalWidth(t *testing.T) {
	defer func(f func(io.Writer) int) { termWidth = f }(termWidth)
	termWidth = func(io.Writer) int { return 60 }
	var b bytes.Buffer
	Table{Header: []string{"NAME", "DESCRIPTION"}, Rows: [][]string{{"code-tour", strings.Repeat("long words ", 40)}}}.Render(&b, Styled)
	for _, l := range strings.Split(strings.TrimRight(b.String(), "\n"), "\n") {
		if w := lipgloss.Width(l); w > 60 {
			t.Fatalf("line is %d cells wide, terminal is 60:\n%s", w, b.String())
		}
	}
	if !strings.Contains(b.String(), "code-tour") {
		t.Fatalf("cell lost:\n%s", b.String())
	}
}

// Package render writes tables and JSON for the CLI in plain, styled or JSON mode.
package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

// Mode selects the output format.
type Mode int

const (
	Plain Mode = iota
	Styled
	JSON
)

// ModeFor picks the mode: --json wins over TTY, a pipe is Plain.
func ModeFor(isTTY, jsonFlag bool) Mode {
	switch {
	case jsonFlag:
		return JSON
	case isTTY:
		return Styled
	}
	return Plain
}

// Table is a header plus rows of cells.
type Table struct {
	Header []string
	Rows   [][]string
}

// sanitize drops C0/C1 control characters, ESC and format (Cf) runes from untrusted cell text;
// tab and newline become a space so cells stay on one line.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n':
			return ' '
		case unicode.IsControl(r) || unicode.Is(unicode.Cf, r): // Cf: bidi overrides, zero-width chars
			return -1
		}
		return r
	}, s)
}

func clean(in []string) []string {
	out := make([]string, len(in))
	for i, c := range in {
		out[i] = sanitize(c)
	}
	return out
}

// Render writes the table. JSON mode is not a table format; callers use WriteJSON, so it renders as Plain.
func (t Table) Render(w io.Writer, m Mode) {
	header := clean(t.Header)
	rows := make([][]string, len(t.Rows))
	for i, r := range t.Rows {
		rows[i] = clean(r)
	}
	if m == Styled {
		tb := table.New().Headers(header...).Rows(rows...).
			StyleFunc(func(row, _ int) lipgloss.Style {
				if row == table.HeaderRow {
					return lipgloss.NewStyle().Bold(true).Padding(0, 1)
				}
				return lipgloss.NewStyle().Padding(0, 1)
			})
		// lipgloss.Fprintln downsamples colors to what the writer supports.
		_, _ = lipgloss.Fprintln(w, tb.String())
		return
	}
	widths := make([]int, len(header))
	for _, r := range append([][]string{header}, rows...) {
		for i, c := range r {
			if i < len(widths) && len([]rune(c)) > widths[i] {
				widths[i] = len([]rune(c))
			}
		}
	}
	line := func(r []string) {
		var b strings.Builder
		for i, c := range r {
			if i > 0 {
				b.WriteString("  ")
			}
			if i < len(r)-1 && i < len(widths) {
				c += strings.Repeat(" ", widths[i]-len([]rune(c)))
			}
			b.WriteString(c)
		}
		_, _ = fmt.Fprintln(w, b.String())
	}
	line(header)
	for _, r := range rows {
		line(r)
	}
}

// WriteJSON writes v (a struct or map) as indented JSON with "schema_version": 1 as the first key.
func WriteJSON(w io.Writer, v any) error {
	var enc bytes.Buffer
	e := json.NewEncoder(&enc)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		return err
	}
	raw := bytes.TrimSpace(enc.Bytes())
	if len(raw) < 2 || raw[0] != '{' {
		return errors.New("render: JSON output must be an object")
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return err
	}
	if _, dup := keys["schema_version"]; dup {
		return errors.New(`render: value already has a "schema_version" key`)
	}
	body := `{"schema_version":1`
	if len(raw) > 2 {
		body += ","
	}
	var out bytes.Buffer
	if err := json.Indent(&out, append([]byte(body), raw[1:]...), "", "  "); err != nil {
		return err
	}
	out.WriteByte('\n')
	_, err := w.Write(out.Bytes())
	return err
}

// Sanitize strips control characters from untrusted text, as table cells do.
func Sanitize(s string) string { return sanitize(s) }

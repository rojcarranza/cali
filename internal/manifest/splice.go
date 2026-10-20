package manifest

import (
	"bytes"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// splice rewrites only the lines of the skill entries that m adds, changes or
// removes, and copies every other line of existing byte for byte. yaml.v3
// re-encodes a whole document with normalized whitespace (inline-comment
// spacing, flow lists) and moves foot comments, so a full re-encode would
// touch unrelated lines. updated is existing's tree after update(m), used to
// encode new and changed entries. ok is false when the change is not limited
// to skill entries or the file has a shape splice does not handle; the caller
// then re-encodes the whole document.
func splice(existing []byte, updated *yaml.Node, m Manifest) ([]byte, bool) {
	var old Manifest
	var orig yaml.Node
	if yaml.Unmarshal(existing, &orig) != nil || orig.Decode(&old) != nil {
		return nil, false
	}
	if old.Version != m.Version || !slices.Equal(old.Targets, m.Targets) || !slices.Equal(old.Sources, m.Sources) {
		return nil, false
	}
	oldSkills := skillsMapping(&orig)
	newSkills := skillsMapping(updated)
	if oldSkills == nil || newSkills == nil || len(oldSkills.Content) == 0 {
		return nil, false
	}
	text := string(existing)
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	if !strings.HasSuffix(text, "\n") {
		text += nl
	}
	lines := strings.SplitAfter(text, "\n")
	lines = lines[:len(lines)-1] // SplitAfter leaves an empty string after the final newline

	type edit struct {
		start, end int // 0-based line range [start, end) to replace
		with       []string
	}
	var edits []edit
	indent := strings.Repeat(" ", oldSkills.Content[0].Column-1)
	last := 0
	for i := 0; i+1 < len(oldSkills.Content); i += 2 {
		key, val := oldSkills.Content[i], oldSkills.Content[i+1]
		start, end := key.Line-1, entryEnd(lines, key, val)
		last = end
		s, ok := m.Skills[key.Value]
		switch {
		case !ok:
			// Drop the head comment with the entry, as the yaml.Node writer does,
			// unless it heads a group: the next entry has no comment of its own.
			grouped := i+2 < len(oldSkills.Content) && oldSkills.Content[i+2].HeadComment == ""
			if n := strings.Count(key.HeadComment, "\n") + 1; key.HeadComment != "" && !grouped && start >= n && allComments(lines[start-n:start]) {
				start -= n
			}
			edits = append(edits, edit{start, end, nil})
		case !sameSkill(old.Skills[key.Value], s):
			before := encodeEntry(key, val, indent, nl)
			after := encodeEntry(key, valueOf(newSkills, key.Value), indent, nl)
			if len(before) != end-start {
				edits = append(edits, edit{start, end, after})
				break
			}
			// Same shape as the file: replace only the lines that differ.
			p := 0
			for p < len(before) && p < len(after) && before[p] == after[p] {
				p++
			}
			q := 0
			for q < len(before)-p && q < len(after)-p && before[len(before)-1-q] == after[len(after)-1-q] {
				q++
			}
			edits = append(edits, edit{start + p, end - q, after[p : len(after)-q]})
		}
	}
	// New entries go after the last existing entry, before any trailing comments.
	var added []string
	for i := 0; i+1 < len(newSkills.Content); i += 2 {
		if _, ok := old.Skills[newSkills.Content[i].Value]; !ok {
			added = append(added, encodeEntry(newSkills.Content[i], newSkills.Content[i+1], indent, nl)...)
		}
	}
	if len(added) > 0 {
		edits = append(edits, edit{last, last, added})
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start > edits[j].start })
	for _, e := range edits {
		lines = slices.Replace(lines, e.start, e.end, e.with...)
	}
	out := []byte(strings.Join(lines, ""))

	// Safety net: the spliced text must decode to exactly m.
	var got Manifest
	if yaml.Unmarshal(out, &got) != nil || !sameManifest(got, m) {
		return nil, false
	}
	return out, true
}

// skillsMapping returns the block-style skills mapping of a document, or nil.
func skillsMapping(doc *yaml.Node) *yaml.Node {
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	v := valueOf(doc.Content[0], "skills")
	if v == nil || v.Kind != yaml.MappingNode || v.Style&yaml.FlowStyle != 0 {
		return nil
	}
	return v
}

// entryEnd returns the 0-based line after the last line of the skill entry
// key: val. Lines indented deeper than key after its last value (block scalar
// or flow list continuations, trailing notes) belong to it; comment lines at
// key indent belong to what follows.
func entryEnd(lines []string, key, val *yaml.Node) int {
	end := maxLine(val)
	for end < len(lines) {
		l := lines[end]
		if strings.TrimSpace(l) == "" || len(l)-len(strings.TrimLeft(l, " ")) < key.Column {
			break
		}
		end++
	}
	return end
}

func maxLine(n *yaml.Node) int {
	m := n.Line
	for _, c := range n.Content {
		m = max(m, maxLine(c))
	}
	return m
}

func allComments(lines []string) bool {
	for _, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "#") {
			return false
		}
	}
	return true
}

// encodeEntry encodes key: val as lines indented by indent. The key's head and
// foot comments lie outside the entry's lines in the file (above it, and at key
// indent after it), so they are left out; comments inside the entry are kept.
func encodeEntry(key, val *yaml.Node, indent, nl string) []string {
	k, v := *key, *val
	k.HeadComment, k.FootComment, v.FootComment = "", "", ""
	pair := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{&k, &v}}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if enc.Encode(pair) != nil || enc.Close() != nil {
		return nil // splice's safety net rejects the result
	}
	lines := strings.SplitAfter(buf.String(), "\n")
	lines = lines[:len(lines)-1]
	for i, l := range lines {
		lines[i] = indent + strings.TrimSuffix(l, "\n") + nl
	}
	return lines
}

func sameSkill(a, b Skill) bool {
	return a.Source == b.Source && a.Ref == b.Ref && slices.Equal(a.Targets, b.Targets) && slices.Equal(a.Requires, b.Requires)
}

func sameManifest(a, b Manifest) bool {
	if a.Version != b.Version || !slices.Equal(a.Targets, b.Targets) || !slices.Equal(a.Sources, b.Sources) || len(a.Skills) != len(b.Skills) {
		return false
	}
	for n, s := range b.Skills {
		if t, ok := a.Skills[n]; !ok || !sameSkill(t, s) {
			return false
		}
	}
	return true
}

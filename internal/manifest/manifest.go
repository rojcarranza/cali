// Package manifest reads and writes cali.yaml. Reads are strict (unknown keys
// are errors). Writes edit the existing yaml.Node tree so hand-written
// comments survive (ADR 0002).
package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/rojcarranza/cali/internal/source"
)

const currentVersion = 1

// Skill is one manifest entry, keyed by install name in Manifest.Skills.
type Skill struct {
	Source   string   `yaml:"source"`
	Ref      string   `yaml:"ref,omitempty"`
	Targets  []string `yaml:"targets,omitempty"`
	Requires []string `yaml:"requires,omitempty"`
}

// EffectiveRef is the ref sync, update and status use for s: its own ref,
// else the @ref of its source spec (empty means the default branch).
func (s Skill) EffectiveRef(spec source.Spec) string {
	if s.Ref != "" {
		return s.Ref
	}
	return spec.Ref
}

// Manifest is the in-memory form of cali.yaml.
type Manifest struct {
	Version int              `yaml:"version"`
	Targets []string         `yaml:"targets,omitempty"`
	Sources []string         `yaml:"sources,omitempty"`
	Skills  map[string]Skill `yaml:"skills"`
}

var (
	nameRe   = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)
	deviceRe = regexp.MustCompile(`^(con|prn|aux|nul|com[1-9]|lpt[1-9])$`) // Windows device names
)

// SkillNameValid reports whether name is a legal install name: lowercase
// letters, digits and hyphens, 1 to 64 characters, and not a Windows device
// name (con, prn, aux, nul, com1-9, lpt1-9).
func SkillNameValid(name string) bool { return nameRe.MatchString(name) && !deviceRe.MatchString(name) }

// TargetsFor returns the skill's own targets if set, else the default targets.
func (m Manifest) TargetsFor(name string) []string {
	if s, ok := m.Skills[name]; ok && len(s.Targets) > 0 {
		return s.Targets
	}
	return m.Targets
}

// Read loads and validates the manifest at path. Unknown keys are errors.
func Read(path string) (Manifest, error) { return ReadWith(path, source.Options{}) }

// ReadWith is Read with o passed to source.ParseWith when sources are
// validated (o.AllowFile admits file:// test fixtures).
func ReadWith(path string, o source.Options) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read manifest %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		if errors.Is(err, io.EOF) {
			return Manifest{}, fmt.Errorf("parse manifest %s: file is empty", path)
		}
		return Manifest{}, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if err := m.validate(o); err != nil {
		return Manifest{}, fmt.Errorf("manifest %s: %w", path, err)
	}
	if m.Skills == nil {
		m.Skills = map[string]Skill{}
	}
	return m, nil
}

func (m Manifest) validate(o source.Options) error {
	if m.Version != currentVersion {
		return fmt.Errorf("unsupported version %d (want %d)", m.Version, currentVersion)
	}
	if err := validTargets(m.Targets); err != nil {
		return fmt.Errorf("targets: %w", err)
	}
	for i, s := range m.Sources {
		if _, err := source.ParseWith(s, o); err != nil {
			return fmt.Errorf("sources[%d]: %w", i, err)
		}
	}
	names := make([]string, 0, len(m.Skills))
	for n := range m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := m.Skills[n]
		if !SkillNameValid(n) {
			return fmt.Errorf("skill name %q: want lowercase letters, digits and hyphens, 1-64 characters", n)
		}
		if s.Source == "" {
			return fmt.Errorf("skill %s: source is required", n)
		}
		if _, err := source.ParseWith(s.Source, o); err != nil {
			return fmt.Errorf("skill %s: source: %w", n, err)
		}
		if err := validTargets(s.Targets); err != nil {
			return fmt.Errorf("skill %s: targets: %w", n, err)
		}
	}
	return nil
}

func validTargets(ts []string) error {
	for _, t := range ts {
		if t != "claude" && t != "copilot" {
			return fmt.Errorf("unknown target %q (want claude or copilot)", t)
		}
	}
	return nil
}

// Write atomically replaces the manifest at path. If the file exists its
// yaml.Node tree is updated from m so comments are kept; otherwise the file
// is marshalled fresh. m is validated first and nothing is written if invalid.
func Write(path string, m Manifest) error { return WriteWith(path, m, source.Options{}) }

// WriteWith is Write with o passed to source.ParseWith during validation.
func WriteWith(path string, m Manifest, o source.Options) (err error) {
	if err := m.validate(o); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	out, err := render(path, m)
	if err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cali.yaml-*.tmp")
	if err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	name := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close() // may already be closed; the error is irrelevant here
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(out); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	// CreateTemp makes the file 0600; the manifest is a committed, shared file.
	if err = os.Chmod(name, 0o644); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	if err = os.Rename(name, path); err != nil {
		return fmt.Errorf("write manifest %s: %w", path, err)
	}
	return nil
}

func render(path string, m Manifest) ([]byte, error) {
	var root yaml.Node
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if m.Skills == nil {
			m.Skills = map[string]Skill{}
		}
		if err := root.Encode(m); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		if err := yaml.Unmarshal(existing, &root); err != nil {
			return nil, err
		}
		if err := update(&root, m); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// update edits the parsed document in place so that it describes m.
func update(root *yaml.Node, m Manifest) error {
	if root.Kind != yaml.DocumentNode || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return errors.New("existing file is not a YAML mapping")
	}
	top := root.Content[0]
	setScalar(top, "version", fmt.Sprint(m.Version), "!!int")
	setList(top, "targets", m.Targets, true)
	setList(top, "sources", m.Sources, false)

	skills := ensureMapping(top, "skills")
	// Drop entries that are no longer in m.
	kept := skills.Content[:0]
	for i := 0; i+1 < len(skills.Content); i += 2 {
		if _, ok := m.Skills[skills.Content[i].Value]; ok {
			kept = append(kept, skills.Content[i], skills.Content[i+1])
		}
	}
	skills.Content = kept
	names := make([]string, 0, len(m.Skills))
	for n := range m.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := m.Skills[n]
		entry := valueOf(skills, n)
		if entry == nil || entry.Kind != yaml.MappingNode {
			var node yaml.Node
			if err := node.Encode(s); err != nil {
				return err
			}
			markFlow(&node)
			if entry != nil { // present but not a mapping: replace, keep comments
				copyComments(&node, entry)
				*entry = node
			} else {
				skills.Content = append(skills.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: n}, &node)
			}
			continue
		}
		setScalar(entry, "source", s.Source, "!!str")
		setScalar(entry, "ref", s.Ref, "!!str")
		setList(entry, "targets", s.Targets, true)
		setList(entry, "requires", s.Requires, true)
	}
	return nil
}

// markFlow renders targets and requires lists of a freshly encoded skill in
// flow style, as in the spec example.
func markFlow(skill *yaml.Node) {
	for i := 0; i+1 < len(skill.Content); i += 2 {
		if v := skill.Content[i+1]; v.Kind == yaml.SequenceNode {
			v.Style |= yaml.FlowStyle
		}
	}
}

func valueOf(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func removeKey(mapping *yaml.Node, key string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = slices.Delete(mapping.Content, i, i+2)
			return
		}
	}
}

func copyComments(dst, src *yaml.Node) {
	dst.HeadComment, dst.LineComment, dst.FootComment = src.HeadComment, src.LineComment, src.FootComment
}

// ensureMapping returns the mapping stored under key, creating or replacing
// the value (e.g. an empty "skills:" null) when it is not a mapping.
func ensureMapping(parent *yaml.Node, key string) *yaml.Node {
	v := valueOf(parent, key)
	if v == nil {
		v = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
		return v
	}
	if v.Kind != yaml.MappingNode {
		old := *v
		*v = yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		copyComments(v, &old)
	}
	v.Style &^= yaml.FlowStyle // adding entries to "{}" must produce block style
	return v
}

// setScalar sets key to val, or removes it when val is empty. An unchanged
// value is left untouched so its style and comments stay as written.
func setScalar(mapping *yaml.Node, key, val, tag string) {
	if val == "" {
		removeKey(mapping, key)
		return
	}
	if v := valueOf(mapping, key); v != nil {
		if v.Kind == yaml.ScalarNode && v.Value == val {
			return
		}
		old := *v
		*v = yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: val}
		copyComments(v, &old)
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: val})
}

// setList sets key to vals, or removes it when vals is empty. An unchanged
// list is left untouched; a changed one keeps its flow/block style and comments.
func setList(mapping *yaml.Node, key string, vals []string, flow bool) {
	if len(vals) == 0 {
		removeKey(mapping, key)
		return
	}
	v := valueOf(mapping, key)
	if v != nil && v.Kind == yaml.SequenceNode && len(v.Content) == len(vals) {
		same := true
		for i, c := range v.Content {
			if c.Kind != yaml.ScalarNode || c.Value != vals[i] {
				same = false
				break
			}
		}
		if same {
			return
		}
	}
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if flow {
		seq.Style = yaml.FlowStyle
	}
	for _, s := range vals {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s})
	}
	if v == nil {
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, seq)
		return
	}
	if v.Kind == yaml.SequenceNode {
		seq.Style = v.Style & yaml.FlowStyle
	}
	copyComments(seq, v)
	*v = *seq
}

package skillfs

import (
	"bytes"
	"errors"

	"go.yaml.in/yaml/v3"
)

// Frontmatter is the part of a SKILL.md YAML header that Cali reads. Other
// keys are ignored.
type Frontmatter struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Metadata    map[string]string `yaml:"metadata"`
}

// ParseFrontmatter reads the YAML block delimited by "---" lines at the very
// start of a SKILL.md (a UTF-8 BOM and CRLF line endings are tolerated).
// Unknown keys are ignored. Errors never contain file content.
func ParseFrontmatter(data []byte) (Frontmatter, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	first, rest, _ := bytes.Cut(data, []byte("\n"))
	if string(bytes.TrimSuffix(first, []byte("\r"))) != "---" {
		return Frontmatter{}, errors.New("SKILL.md has no frontmatter: want a --- line at the start")
	}
	var block []byte
	found := false
	for len(rest) > 0 {
		line, next, _ := bytes.Cut(rest, []byte("\n"))
		if string(bytes.TrimSuffix(line, []byte("\r"))) == "---" {
			block = data[len(first)+1 : len(data)-len(rest)]
			found = true
			break
		}
		rest = next
	}
	if !found {
		return Frontmatter{}, errors.New("SKILL.md frontmatter has no closing --- line")
	}
	var fm Frontmatter
	if len(bytes.TrimSpace(block)) == 0 {
		return fm, nil
	}
	var node yaml.Node
	if err := yaml.Unmarshal(block, &node); err != nil {
		return Frontmatter{}, errors.New("SKILL.md frontmatter is not valid YAML")
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return Frontmatter{}, errors.New("SKILL.md frontmatter is not a YAML mapping")
	}
	// yaml type errors quote the offending value, so they are replaced.
	if err := node.Decode(&fm); err != nil {
		return Frontmatter{}, errors.New("SKILL.md frontmatter has a field of the wrong type (name, description and metadata values must be strings)")
	}
	return fm, nil
}

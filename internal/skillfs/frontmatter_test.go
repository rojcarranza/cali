package skillfs

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	data := []byte("---\nname: x\ndescription: Does x.\nargument-hint: foo\nmetadata:\n  cali-requires: \"a, b\"\n---\n# Body\n")
	fm, err := ParseFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "x" || fm.Description != "Does x." || fm.Metadata["cali-requires"] != "a, b" {
		t.Fatalf("fm = %+v", fm)
	}
}

func TestParseFrontmatterCRLFAndBOM(t *testing.T) {
	fm, err := ParseFrontmatter([]byte("\xef\xbb\xbf---\r\nname: x\r\ndescription: d\r\n---\r\nbody\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "x" || fm.Description != "d" {
		t.Fatalf("fm = %+v", fm)
	}
}

func TestParseFrontmatterAtEOF(t *testing.T) {
	fm, err := ParseFrontmatter([]byte("---\nname: x\n---"))
	if err != nil || fm.Name != "x" {
		t.Fatalf("fm = %+v, err = %v", fm, err)
	}
}

// Real skills (awesome-copilot's acquire-codebase-knowledge) keep lists in metadata.
func TestParseFrontmatterIgnoresNonStringMetadata(t *testing.T) {
	data := []byte("---\nname: x\nmetadata:\n  version: \"1.3\"\n  enhancements:\n    - a\n    - b\n  nested: {k: v}\n  empty:\n  cali-requires: y\n---\n")
	fm, err := ParseFrontmatter(data)
	if err != nil {
		t.Fatal(err)
	}
	if fm.Metadata["cali-requires"] != "y" || fm.Metadata["version"] != "1.3" || len(fm.Metadata) != 2 {
		t.Fatalf("metadata = %#v", fm.Metadata)
	}
}

func TestParseFrontmatterMetadataAlias(t *testing.T) {
	fm, err := ParseFrontmatter([]byte("---\nname: x\ndeps: &d \"a, b\"\nmetadata:\n  cali-requires: *d\n---\n"))
	if err != nil {
		t.Fatal(err)
	}
	if fm.Metadata["cali-requires"] != "a, b" {
		t.Fatalf("metadata = %#v", fm.Metadata)
	}
	if fm, err = ParseFrontmatter([]byte("---\nname: x\nmetadata:\n  cali-requires: !!binary YQ==\n---\n")); err != nil || fm.Metadata["cali-requires"] != "a" {
		t.Fatalf("binary = %#v, %v", fm.Metadata, err)
	}
}

func TestParseFrontmatterErrors(t *testing.T) {
	const secret = "s3cr3t-content"
	for name, in := range map[string]string{
		"no frontmatter":  "# " + secret + "\nname: x\n",
		"not at start":    "\n---\nname: x\n---\n",
		"unterminated":    "---\nname: " + secret + "\n",
		"not a mapping":   "---\n- " + secret + "\n---\n",
		"bad yaml":        "---\nname: [" + secret + "\n---\n",
		"requires nested": "---\nname: x\nmetadata:\n  cali-requires: {a: " + secret + "}\n---\n",
		"name not string": "---\nname: [" + secret + "]\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseFrontmatter([]byte(in))
			if err == nil {
				t.Fatal("want error")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("error leaks content: %v", err)
			}
		})
	}
}

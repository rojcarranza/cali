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

func TestParseFrontmatterErrors(t *testing.T) {
	const secret = "s3cr3t-content"
	for name, in := range map[string]string{
		"no frontmatter":  "# " + secret + "\nname: x\n",
		"not at start":    "\n---\nname: x\n---\n",
		"unterminated":    "---\nname: " + secret + "\n",
		"not a mapping":   "---\n- " + secret + "\n---\n",
		"bad yaml":        "---\nname: [" + secret + "\n---\n",
		"metadata nested": "---\nname: x\nmetadata:\n  k: {a: " + secret + "}\n---\n",
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

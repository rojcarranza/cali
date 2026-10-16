package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/cli"
)

func TestMainExitsZeroOnVersion(t *testing.T) {
	var out, errb bytes.Buffer

	code := cli.Execute([]string{"--version"}, &out, &errb)

	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errb.String())
	}
	if !strings.Contains(out.String(), "cali") {
		t.Fatalf("stdout = %q, want it to contain %q", out.String(), "cali")
	}
}

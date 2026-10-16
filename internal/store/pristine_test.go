package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rojcarranza/cali/internal/skillfs"
)

const sha = "0123456789abcdef0123456789abcdef01234567"

func tree(body string) skillfs.Tree {
	return skillfs.Tree{
		{Path: "SKILL.md", Data: []byte(body)},
		{Path: "run.sh", Exec: true, Data: []byte("#!/bin/sh\n")},
	}
}

func TestPutGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, ok, err := Get(dir, sha); err != nil || ok {
		t.Fatalf("Get absent = %v, %v; want false, nil", ok, err)
	}
	if err := Put(dir, sha, tree("a\n")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Get(dir, sha)
	if err != nil || !ok {
		t.Fatalf("Get = %v, %v", ok, err)
	}
	if !skillfs.Equal(got, tree("a\n")) {
		t.Fatalf("round trip differs: %+v", got)
	}
	if err := Put(dir, sha, tree("a\n")); err != nil {
		t.Fatalf("idempotent Put: %v", err)
	}
	if err := Put(dir, sha, tree("b\n")); err == nil {
		t.Fatal("want error when existing pristine differs")
	}
	ents, _ := os.ReadDir(filepath.Join(dir, "pristine"))
	if len(ents) != 1 {
		t.Fatalf("leftover entries: %v", ents)
	}
}

func TestPutRejectsBadTreeSHA(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"", "../x", strings.ToUpper(sha), sha[:39], sha + "0", "zz" + sha[2:]} {
		if err := Put(dir, bad, tree("a\n")); err == nil {
			t.Errorf("Put(%q) want error", bad)
		}
		if _, _, err := Get(dir, bad); err == nil {
			t.Errorf("Get(%q) want error", bad)
		}
	}
}

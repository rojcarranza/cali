package gitx

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	return m
}

func TestAuthEnvOnlyGithub(t *testing.T) {
	getenv := func(k string) string {
		if k == "GITHUB_TOKEN" {
			return "tok123"
		}
		return ""
	}
	got := envMap(AuthEnv("github.com", getenv, nil))
	if got["GIT_CONFIG_COUNT"] != "1" ||
		got["GIT_CONFIG_KEY_0"] != "http.https://github.com/.extraheader" {
		t.Fatalf("env = %v", got)
	}
	want := "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:tok123"))
	if got["GIT_CONFIG_VALUE_0"] != want {
		t.Fatalf("value = %q", got["GIT_CONFIG_VALUE_0"])
	}
	for _, e := range AuthEnv("github.com", getenv, nil) {
		if strings.Contains(e, "tok123") {
			t.Fatalf("raw token in %q", e)
		}
	}
	if env := AuthEnv("example.com", getenv, nil); len(env) != 0 {
		t.Fatalf("non-github env = %v", env)
	}
}

func TestScrubRemovesCredentials(t *testing.T) {
	env := AuthEnv("github.com", func(string) string { return "s3cr3t-token" }, nil)
	cred := strings.TrimPrefix(envMap(env)["GIT_CONFIG_VALUE_0"], "AUTHORIZATION: basic ")
	in := "fatal: header AUTHORIZATION: basic " + cred + " and s3cr3t-token seen"
	out := scrub(in, env)
	if strings.Contains(out, "s3cr3t-token") || strings.Contains(out, cred) {
		t.Fatalf("scrub left credentials: %q", out)
	}
}

func TestAuthEnvTokenOrder(t *testing.T) {
	val := func(env []string) string { return envMap(env)["GIT_CONFIG_VALUE_0"] }
	enc := func(tok string) string {
		return "AUTHORIZATION: basic " + base64.StdEncoding.EncodeToString([]byte("x-access-token:"+tok))
	}
	both := func(k string) string {
		return map[string]string{"GITHUB_TOKEN": "a", "GH_TOKEN": "b"}[k]
	}
	if got := val(AuthEnv("github.com", both, nil)); got != enc("a") {
		t.Errorf("GITHUB_TOKEN should win, got %q", got)
	}
	gh := func(k string) string { return map[string]string{"GH_TOKEN": "b"}[k] }
	if got := val(AuthEnv("github.com", gh, nil)); got != enc("b") {
		t.Errorf("GH_TOKEN, got %q", got)
	}
	none := func(string) string { return "" }
	if got := val(AuthEnv("github.com", none, func() (string, error) { return "c\n", nil })); got != enc("c") {
		t.Errorf("gh fallback, got %q", got)
	}
	if env := AuthEnv("github.com", none, func() (string, error) { return "", errors.New("no gh") }); len(env) != 0 {
		t.Errorf("env = %v", env)
	}
	if env := AuthEnv("github.com", none, nil); len(env) != 0 {
		t.Errorf("env = %v", env)
	}
}

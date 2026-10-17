package gitx

import (
	"encoding/base64"
	"strings"
)

// AuthEnv returns git environment entries that authenticate https requests to
// github.com with a token from GITHUB_TOKEN, GH_TOKEN or ghToken (in that
// order). It returns nil for any other host or when no token is found. The
// token travels only in the environment, never in argv or files.
func AuthEnv(host string, getenv func(string) string, ghToken func() (string, error)) []string {
	if host != "github.com" {
		return nil
	}
	token := strings.TrimSpace(getenv("GITHUB_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(getenv("GH_TOKEN"))
	}
	if token == "" && ghToken != nil {
		if t, err := ghToken(); err == nil {
			token = strings.TrimSpace(t)
		}
	}
	if token == "" {
		return nil
	}
	cred := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	return []string{
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.https://github.com/.extraheader",
		"GIT_CONFIG_VALUE_0=AUTHORIZATION: basic " + cred,
	}
}

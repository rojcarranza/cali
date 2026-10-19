package dirs

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func check(t *testing.T, got, want Dirs) {
	t.Helper()
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestResolveUnix(t *testing.T) {
	want := Dirs{
		Config: "/h/.config/cali",
		Data:   "/h/.local/share/cali",
		Cache:  "/h/.cache/cali",
		Home:   "/h",
	}
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			check(t, Resolve(goos, envOf(nil), "/h", FS{}), want)
		})
	}
}

func TestResolveXDG(t *testing.T) {
	env := envOf(map[string]string{
		"XDG_CONFIG_HOME": "/x/c",
		"XDG_DATA_HOME":   "/x/d",
		"XDG_CACHE_HOME":  "/x/k",
	})
	check(t, Resolve("linux", env, "/h", FS{}), Dirs{
		Config: "/x/c/cali",
		Data:   "/x/d/cali",
		Cache:  "/x/k/cali",
		Home:   "/h",
	})
}

func TestResolveCaliOverrides(t *testing.T) {
	env := envOf(map[string]string{
		"CALI_CONFIG_DIR": "/o/c",
		"CALI_DATA_DIR":   "/o/d",
		"CALI_CACHE_DIR":  "/o/k",
		"XDG_CONFIG_HOME": "/x/c",
		"XDG_DATA_HOME":   "/x/d",
		"XDG_CACHE_HOME":  "/x/k",
	})
	check(t, Resolve("linux", env, "/h", FS{}), Dirs{
		Config: "/o/c",
		Data:   "/o/d",
		Cache:  "/o/k",
		Home:   "/h",
	})
}

func TestResolveWindows(t *testing.T) {
	env := envOf(map[string]string{
		"APPDATA":      `C:\A`,
		"LOCALAPPDATA": `C:\L`,
	})
	check(t, Resolve("windows", env, `C:\Users\r`, FS{}), Dirs{
		Config: `C:\Users\r\.config\cali`,
		Data:   `C:\L\cali\data`,
		Cache:  `C:\L\cali\cache`,
		Home:   `C:\Users\r`,
	})
}

func TestResolveWindowsOverrides(t *testing.T) {
	env := envOf(map[string]string{
		"APPDATA":        `C:\A`,
		"LOCALAPPDATA":   `C:\L`,
		"CALI_DATA_DIR":  `D:\data`,
		"CALI_CACHE_DIR": `D:\cache`,
	})
	check(t, Resolve("windows", env, `C:\Users\r`, FS{}), Dirs{
		Config: `C:\Users\r\.config\cali`,
		Data:   `D:\data`,
		Cache:  `D:\cache`,
		Home:   `C:\Users\r`,
	})
}

// fsOf reports the listed paths as existing and globs to matches.
func fsOf(existing []string, matches ...string) FS {
	set := map[string]bool{}
	for _, p := range existing {
		set[p] = true
	}
	return FS{
		Exists: func(p string) bool { return set[p] },
		Glob:   func(string) ([]string, error) { return matches, nil },
	}
}

func TestResolveWSLFallback(t *testing.T) {
	env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	win := "/mnt/c/Users/r/.config/cali/cali.yaml"
	got := Resolve("linux", env, "/h", fsOf(nil, win))
	if got.Config != "/mnt/c/Users/r/.config/cali" || got.Note != "" {
		t.Errorf("got %+v, want Windows home config", got)
	}
}

func TestResolveWSLOwnManifestWins(t *testing.T) {
	env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	fs := fsOf([]string{"/h/.config/cali/cali.yaml"}, "/mnt/c/Users/r/.config/cali/cali.yaml")
	if got := Resolve("linux", env, "/h", fs).Config; got != "/h/.config/cali" {
		t.Errorf("Config = %s, want /h/.config/cali", got)
	}
}

func TestResolveWSLSeveralWindowsHomes(t *testing.T) {
	env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	fs := fsOf(nil, "/mnt/c/Users/a/.config/cali/cali.yaml", "/mnt/c/Users/b/.config/cali/cali.yaml")
	got := Resolve("linux", env, "/h", fs)
	if got.Config != "/h/.config/cali" {
		t.Errorf("Config = %s, want /h/.config/cali", got.Config)
	}
	if !strings.Contains(got.Note, "CALI_CONFIG_DIR") || !strings.Contains(got.Note, "/mnt/c/Users/b/.config/cali") {
		t.Errorf("Note = %q, want both homes and CALI_CONFIG_DIR", got.Note)
	}
}

func TestResolveWSLOverridesSkipFallback(t *testing.T) {
	win := "/mnt/c/Users/r/.config/cali/cali.yaml"
	for k, want := range map[string]string{"CALI_CONFIG_DIR": "/o/c", "XDG_CONFIG_HOME": "/x/c/cali"} {
		v := strings.TrimSuffix(want, "/cali")
		env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu", k: v})
		if got := Resolve("linux", env, "/h", fsOf(nil, win)).Config; got != want {
			t.Errorf("%s: Config = %s, want %s", k, got, want)
		}
	}
}

func TestResolveNotWSLNoFallback(t *testing.T) {
	win := "/mnt/c/Users/r/.config/cali/cali.yaml"
	if got := Resolve("linux", envOf(nil), "/h", fsOf(nil, win)).Config; got != "/h/.config/cali" {
		t.Errorf("Config = %s, want /h/.config/cali", got)
	}
}

func TestResolveWindowsLegacyAppData(t *testing.T) {
	env := envOf(map[string]string{"APPDATA": `C:\A`})
	fs := fsOf([]string{`C:\A\cali\cali.yaml`})
	if got := Resolve("windows", env, `C:\Users\r`, fs).Config; got != `C:\A\cali` {
		t.Errorf("Config = %s, want legacy %%APPDATA%%", got)
	}
	fs = fsOf([]string{`C:\A\cali\cali.yaml`, `C:\Users\r\.config\cali`})
	if got := Resolve("windows", env, `C:\Users\r`, fs).Config; got != `C:\Users\r\.config\cali` {
		t.Errorf("Config = %s, want home config once it exists", got)
	}
}

func TestResolveWindowsIgnoresXDG(t *testing.T) {
	env := envOf(map[string]string{"XDG_CONFIG_HOME": "/c/Users/r/.config"})
	if got := Resolve("windows", env, `C:\Users\r`, FS{}).Config; got != `C:\Users\r\.config\cali` {
		t.Errorf("Config = %s, want home config", got)
	}
}

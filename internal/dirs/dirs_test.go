package dirs

import (
	"path"
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
		Glob: func(pattern string) ([]string, error) {
			var out []string
			for _, m := range matches {
				if ok, _ := path.Match(pattern, m); ok {
					out = append(out, m)
				}
			}
			return out, nil
		},
	}
}

func TestWSLConfigFallback(t *testing.T) {
	env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	win := "/mnt/c/Users/r/.config/cali/cali.yaml"
	for _, win := range []string{win, "/mnt/d/Users/r/.config/cali/cali.yaml", "/c/Users/r/.config/cali/cali.yaml"} {
		dir, note := WSLConfig(env, "/h/.config/cali", fsOf(nil, win))
		if want := strings.TrimSuffix(win, "/cali.yaml"); dir != want || note != "" {
			t.Errorf("%s: got %s %q, want %s", win, dir, note, want)
		}
	}
}

func TestWSLConfigOwnManifestWins(t *testing.T) {
	env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	fs := fsOf([]string{"/h/.config/cali/cali.yaml"}, "/mnt/c/Users/r/.config/cali/cali.yaml")
	if got, _ := WSLConfig(env, "/h/.config/cali", fs); got != "/h/.config/cali" {
		t.Errorf("Config = %s, want /h/.config/cali", got)
	}
}

func TestWSLConfigSeveralWindowsHomes(t *testing.T) {
	env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	fs := fsOf(nil, "/mnt/c/Users/a/.config/cali/cali.yaml", "/mnt/c/Users/b/.config/cali/cali.yaml")
	dir, note := WSLConfig(env, "/h/.config/cali", fs)
	if dir != "/h/.config/cali" {
		t.Errorf("Config = %s, want /h/.config/cali", dir)
	}
	if !strings.Contains(note, "CALI_CONFIG_DIR") || !strings.Contains(note, "/mnt/c/Users/b/.config/cali") {
		t.Errorf("note = %q, want both homes and CALI_CONFIG_DIR", note)
	}
}

func TestWSLConfigOverridesSkipFallback(t *testing.T) {
	win := "/mnt/c/Users/r/.config/cali/cali.yaml"
	for k, want := range map[string]string{"CALI_CONFIG_DIR": "/o/c", "XDG_CONFIG_HOME": "/x/c/cali"} {
		v := strings.TrimSuffix(want, "/cali")
		env := envOf(map[string]string{"WSL_DISTRO_NAME": "Ubuntu", k: v})
		if got, _ := WSLConfig(env, want, fsOf(nil, win)); got != want {
			t.Errorf("%s: Config = %s, want %s", k, got, want)
		}
	}
}

func TestWSLConfigNotWSL(t *testing.T) {
	win := "/mnt/c/Users/r/.config/cali/cali.yaml"
	if got, _ := WSLConfig(envOf(nil), "/h/.config/cali", fsOf(nil, win)); got != "/h/.config/cali" {
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

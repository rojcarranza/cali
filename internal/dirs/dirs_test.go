package dirs

import "testing"

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
			check(t, Resolve(goos, envOf(nil), "/h"), want)
		})
	}
}

func TestResolveXDG(t *testing.T) {
	env := envOf(map[string]string{
		"XDG_CONFIG_HOME": "/x/c",
		"XDG_DATA_HOME":   "/x/d",
		"XDG_CACHE_HOME":  "/x/k",
	})
	check(t, Resolve("linux", env, "/h"), Dirs{
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
	check(t, Resolve("linux", env, "/h"), Dirs{
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
	check(t, Resolve("windows", env, `C:\Users\r`), Dirs{
		Config: `C:\A\cali`,
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
	check(t, Resolve("windows", env, `C:\Users\r`), Dirs{
		Config: `C:\A\cali`,
		Data:   `D:\data`,
		Cache:  `D:\cache`,
		Home:   `C:\Users\r`,
	})
}

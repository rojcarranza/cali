// Package dirs resolves Cali's per-user config, data and cache directories
// (ADR 0005, ADR 0009).
package dirs

import "strings"

// Dirs holds the resolved per-user directories and the home directory.
type Dirs struct {
	Config, Data, Cache, Home string
	// Note says why the WSL fallback was passed over, for list -g.
	Note string
}

// FS is what Resolve asks the file system. A nil func means nothing exists
// and nothing matches.
type FS struct {
	Exists func(path string) bool
	Glob   func(pattern string) ([]string, error)
}

// wslConfigGlob finds global manifests in Windows homes from WSL, so cali in
// WSL and cali.exe in Git Bash share one (ADR 0009).
const wslConfigGlob = "/mnt/c/Users/*/.config/cali/cali.yaml"

// Resolve computes the directories for goos. env is typically os.Getenv and
// home os.UserHomeDir(). Paths are joined with explicit separators (never the
// host's filepath) so results are the same on any host. Empty environment
// values count as unset.
func Resolve(goos string, env func(string) string, home string, fs FS) Dirs {
	if goos == "windows" {
		return resolveWindows(env, home, fs)
	}
	d := Dirs{
		Config: pick(env, "CALI_CONFIG_DIR", "XDG_CONFIG_HOME", home, ".config", "/"),
		Data:   pick(env, "CALI_DATA_DIR", "XDG_DATA_HOME", home, ".local/share", "/"),
		Cache:  pick(env, "CALI_CACHE_DIR", "XDG_CACHE_HOME", home, ".cache", "/"),
		Home:   home,
	}
	if env("WSL_DISTRO_NAME") != "" && env("CALI_CONFIG_DIR") == "" && env("XDG_CONFIG_HOME") == "" &&
		!fs.exists(d.Config+"/cali.yaml") {
		switch m := fs.glob(wslConfigGlob); len(m) {
		case 0:
		case 1:
			d.Config = strings.TrimSuffix(m[0], "/cali.yaml")
		default:
			d.Note = "several Windows homes hold a global cali.yaml (" + strings.Join(m, ", ") +
				"); set CALI_CONFIG_DIR to the one to use"
		}
	}
	return d
}

func (fs FS) exists(p string) bool { return fs.Exists != nil && fs.Exists(p) }

func (fs FS) glob(pattern string) []string {
	if fs.Glob == nil {
		return nil
	}
	m, _ := fs.Glob(pattern) // the pattern is constant, so ErrBadPattern cannot happen
	return m
}

func pick(env func(string) string, caliVar, xdgVar, home, rel, sep string) string {
	if v := env(caliVar); v != "" {
		return v
	}
	if v := env(xdgVar); v != "" {
		return join(sep, v, "cali")
	}
	return join(sep, home, rel, "cali")
}

func resolveWindows(env func(string) string, home string, fs FS) Dirs {
	roaming := env("APPDATA")
	if roaming == "" {
		roaming = join(`\`, home, "AppData", "Roaming")
	}
	local := env("LOCALAPPDATA")
	if local == "" {
		local = join(`\`, home, "AppData", "Local")
	}
	d := Dirs{
		Config: join(`\`, home, ".config", "cali"),
		Data:   join(`\`, local, "cali", "data"),
		Cache:  join(`\`, local, "cali", "cache"),
		Home:   home,
	}
	// A global manifest from before ADR 0009 keeps working until the new dir exists.
	if legacy := join(`\`, roaming, "cali"); !fs.exists(d.Config) && fs.exists(legacy+`\cali.yaml`) {
		d.Config = legacy
	}
	if v := env("CALI_CONFIG_DIR"); v != "" {
		d.Config = v
	}
	if v := env("CALI_DATA_DIR"); v != "" {
		d.Data = v
	}
	if v := env("CALI_CACHE_DIR"); v != "" {
		d.Cache = v
	}
	return d
}

func join(sep string, parts ...string) string {
	return strings.Join(parts, sep)
}

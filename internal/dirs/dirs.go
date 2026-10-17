// Package dirs resolves Cali's per-user config, data and cache directories
// (ADR 0005).
package dirs

import "strings"

// Dirs holds the resolved per-user directories and the home directory.
type Dirs struct {
	Config, Data, Cache, Home string
}

// Resolve computes the directories for goos. It is pure: env is typically
// os.Getenv and home os.UserHomeDir(). Paths are joined with explicit
// separators (never the host's filepath) so results are the same on any host.
// Empty environment values count as unset.
func Resolve(goos string, env func(string) string, home string) Dirs {
	if goos == "windows" {
		return resolveWindows(env, home)
	}
	return Dirs{
		Config: pick(env, "CALI_CONFIG_DIR", "XDG_CONFIG_HOME", home, ".config", "/"),
		Data:   pick(env, "CALI_DATA_DIR", "XDG_DATA_HOME", home, ".local/share", "/"),
		Cache:  pick(env, "CALI_CACHE_DIR", "XDG_CACHE_HOME", home, ".cache", "/"),
		Home:   home,
	}
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

func resolveWindows(env func(string) string, home string) Dirs {
	roaming := env("APPDATA")
	if roaming == "" {
		roaming = join(`\`, home, "AppData", "Roaming")
	}
	local := env("LOCALAPPDATA")
	if local == "" {
		local = join(`\`, home, "AppData", "Local")
	}
	d := Dirs{
		Config: join(`\`, roaming, "cali"),
		Data:   join(`\`, local, "cali", "data"),
		Cache:  join(`\`, local, "cali", "cache"),
		Home:   home,
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

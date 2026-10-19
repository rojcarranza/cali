# ADR 0009: Home-relative config directory on every OS

Status: accepted (2026-10-19). Supersedes the `<config>` row of ADR 0005.

## Context

ADR 0005 puts `<config>` at `~/.config/cali` on macOS, Linux and WSL, and at `%APPDATA%\cali` on Windows. On a Windows machine with both shells, `cali.exe` in Git Bash and `cali` in WSL then keep two global manifests, because WSL has its own home: a skill added from one is unknown to the other unless `CALI_MANIFEST` or `CALI_CONFIG_DIR` is set by hand (cali#35).

bam-cli, used on the same machines, already shares one presets file between Git Bash, WSL and macOS by using the same home-relative path everywhere and, in WSL only, falling back to the file in the Windows home (bam-cli `internal/config/presets.go`). Its machine config, however, follows `adrg/xdg` (`~/Library/Application Support` on macOS, `%LOCALAPPDATA%` on Windows). Cali and bam should follow one rule, so a user looks for both in the same place.

## Decision

`<config>` is resolved, in order:

1. `$CALI_CONFIG_DIR`.
2. Not on Windows: `$XDG_CONFIG_HOME/cali`. On Windows `XDG_CONFIG_HOME` is ignored, since Git Bash may export it as an MSYS path (`/c/...`) that `cali.exe` cannot open.
3. In WSL only (`WSL_DISTRO_NAME` set), when `<home>/.config/cali/cali.yaml` does not exist: the one `/mnt/c/Users/*/.config/cali/` that holds a `cali.yaml`. With several matches Cali uses none of them, stays in the WSL home, and `list -g` says to set `CALI_CONFIG_DIR`.
4. On Windows only, when `<home>\.config\cali` does not exist and `%APPDATA%\cali\cali.yaml` does: `%APPDATA%\cali`, so an existing global manifest keeps working. Cali does not move it.
5. `<home>/.config/cali`, where `<home>` is `os.UserHomeDir()` (`%USERPROFILE%` on Windows, not the MSYS `$HOME`).

The global manifest and lock stay at `<config>/cali.yaml` and `<config>/cali.lock`; `--manifest` and `$CALI_MANIFEST` (ADR 0008) still take precedence over `<config>`.

A first global manifest created from WSL is written in the WSL home, not the Windows one. Writing into the Windows home would need `cmd.exe` and `wslpath` to find it, as bam does; create it from Git Bash first to share it.

`<data>` and `<cache>` are unchanged from ADR 0005. They hold per-machine state (pristine copies, backups, mirrors) and are not shared between WSL and Windows. The rest of ADR 0005 (files, pristine store, write boundaries) still applies.

bam-cli adopts the same rule for its config, credentials and presets, under `bam` instead of `cali`, in its own change.

## Consequences

- Git Bash, PowerShell and WSL on one machine share one global `cali.yaml` and `cali.lock` with no environment variable, as long as WSL has none of its own. The lock holds no absolute paths or per-machine data (`internal/lock/lock.go:3`), so sharing it is safe.
- Global skills are still installed per home (`~/.claude/skills`, `~/.copilot/skills`), so WSL and Windows each have their own copies. After `update -g` on one side, run `cali sync -g` on the other to bring its folders to the shared lock.
- WSL reads a manifest on the Windows drive (`/mnt/c`), which is slower than the WSL filesystem. Cali reads it once per command.
- macOS, Linux and WSL users with a config already at `~/.config/cali` see no change. Windows users keep `%APPDATA%\cali` until they create `%USERPROFILE%\.config\cali`.
- `internal/dirs` gains a WSL check and a glob; tests stub both, as bam does with `PresetsLookup.Glob`.
- No new dependency, datastore or service.

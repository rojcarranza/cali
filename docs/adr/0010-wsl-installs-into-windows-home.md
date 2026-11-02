# ADR 0010: WSL can install into the Windows home

Status: accepted (2026-11-01). Reverses the **WSL** decision in design spec section 6 ("never writes to the Windows profile").

## Context

Design spec section 6 kept the Linux binary in WSL away from the Windows profile. The reason was to avoid `/mnt/c` and `\\wsl$` translation. ADR 0009 already reads the global manifest from the Windows home in WSL. The remaining gap: at work the editors that read skills (VS Code, JetBrains) run on Windows and read `%USERPROFILE%\.copilot\skills`, while cali is often run from WSL, whose `~/.copilot/skills` they never see (SCR-14 piece 4).

## Decision

- `--windows-home` (persistent flag, needs `-g`) makes global installs, status, list, update and remove use the Windows home instead of the WSL home. It is opt-in; without it nothing changes, so the spec's default stays.
- The Windows home is `$CALI_WINDOWS_HOME` when set, else the one profile under `/mnt/*/Users/*` or `/*/Users/*` (Public, Default, Default User and All Users skipped). With none or several, cali stops and names `CALI_WINDOWS_HOME`. The flag outside WSL, without the variable, is an error.
- On Windows (`cali.exe` in Git Bash or PowerShell) the home is already the Windows one, so the flag does nothing there.
- Combine with `--target copilot` (SCR-14 piece 1) to skip the Claude folder.
- `<data>` and `<cache>` (pristine copies, backups) stay in the WSL home, as in ADR 0009; only the install folders move. No new dependency, datastore or service.

## Consequences

- `cali sync -g --windows-home --target copilot` in WSL fills the folders Windows editors read.
- Writes go through the DrvFs mount, which is slower than the WSL filesystem.
- Profile detection by directory glob does not use `cmd.exe` or `wslpath`; other automount roots need `CALI_WINDOWS_HOME`.

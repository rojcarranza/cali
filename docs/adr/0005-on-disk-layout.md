# ADR 0005: On-disk layout and pristine store

Status: accepted (2026-10-16)

## Context

Cali keeps a manifest and lock, installed skill folders, git mirrors, and the unmodified upstream copy of each installed skill (the merge base for 3-way updates). These have different lifetimes: some are committed, some are rebuildable, one must survive a force-pushed upstream. Paths must agree between Git Bash, PowerShell and WSL (design spec sections 2 and 5).

## Decision

Per-user directories, resolved with `os.UserHomeDir()` (on Windows `%USERPROFILE%`, not the MSYS `$HOME`):

| Dir | macOS / Linux / WSL | Windows |
|---|---|---|
| `<config>` | `$CALI_CONFIG_DIR`, else `$XDG_CONFIG_HOME/cali`, else `~/.config/cali` | `%APPDATA%\cali` |
| `<data>` | `$CALI_DATA_DIR`, else `$XDG_DATA_HOME/cali`, else `~/.local/share/cali` | `%LOCALAPPDATA%\cali\data` |
| `<cache>` | `$CALI_CACHE_DIR`, else `$XDG_CACHE_HOME/cali`, else `~/.cache/cali` | `%LOCALAPPDATA%\cali\cache` |

macOS uses the XDG-style paths, not `~/Library/...`, so Mac, Linux and WSL follow one rule.

Files:
- Project scope: `<project>/cali.yaml`, `<project>/cali.lock`, installed skills in `<project>/.claude/skills/<name>` and `<project>/.github/skills/<name>`.
- Global scope: `<config>/cali.yaml`, `<config>/cali.lock`, skills in `~/.claude/skills/<name>` and `~/.copilot/skills/<name>`. `--manifest <path>` overrides the manifest location (in global scope `$CALI_MANIFEST` is its default, ADR 0008); the lock sits next to it.
- `<cache>/git/<host>/<owner>/<repo>.git`: one bare mirror per source. Disposable.
- `<data>/pristine/<tree-sha>/`: plain-file copy of the upstream skill folder for each installed tree SHA, shared across scopes and targets. It lives in `<data>`, not `<cache>`, because it is the merge base and must survive a force-pushed upstream or a deleted mirror. If missing, `sync` rebuilds it from the lock's `commit`; if that commit is gone upstream the skill is reported `base-lost`. A pristine dir is removed only by `remove`, and only when no lock on this machine still references that tree.
- `<data>/backups/<name>/<UTC-timestamp>/`: copy of the folder taken before any update or remove writes.

Cali never edits `.gitignore` and touches nothing outside the target skill folders, `<data>`, `<cache>`, `<config>`, and the project's `cali.yaml` and `cali.lock`.

## Consequences

- Deleting `<cache>` is always safe; deleting `<data>` can cause `base-lost` for skills whose upstream commit has vanished.
- Keying pristine copies by tree SHA deduplicates identical content and ties the base to the lock's identity for the skill.
- Environment overrides make tests hermetic: each test points the three variables at a temp dir.
- Concurrent Cali processes are not locked (deferred); atomic writes via temp file and rename prevent corruption but not lost updates.

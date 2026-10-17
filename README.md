# Cali

Cali is one Go binary that installs, pins, updates and syncs agent skills from any git source. It records where every skill came from in `cali.lock` and merges upstream changes into local edits instead of overwriting them. It never calls an LLM.

## Install

```sh
make install    # macOS, Linux, WSL; on Git Bash without make: sh scripts/install.sh
```

See [SETUP.md](SETUP.md) for what it does and for manual installs.

## Build

```sh
CGO_ENABLED=0 go build -o bin/cali ./cmd/cali
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/cali-darwin-arm64 ./cmd/cali
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/cali.exe ./cmd/cali
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/cali-linux-amd64 ./cmd/cali
```

System `git` must be on PATH (ADR 0001). GitHub tokens come from `GITHUB_TOKEN` or `GH_TOKEN`, else from `gh auth token`.

## Commands

- `cali init`: create `cali.yaml`.
- `cali add <spec>`: install a skill from a git repo (`--skill <name>`, `--all`, `--target claude|copilot` repeatable, `--adopt` to take over an unmanaged folder).
- `cali list`: list installed skills (`--json`).
- `cali show <name>`: show one skill (`--json`).
- `cali find <query>`: search the configured sources and installed skills (`--json`, `--offline`).
- `cali status`: show local edits, newer upstream versions and missing skills (`--json`, `--offline`, `--exit-code`).
- `cali update [names...]`: dry run by default; `--apply` writes it. Conflicts: `--resolve <file>=ours|theirs|merge` (repeatable, one skill), `--take-upstream`, `--keep-mine`, `--accept-recommended`. `--json`. A skill whose ref or folder is gone upstream, or whose upstream folder is invalid, is reported as skipped; the rest is still planned or applied and the exit code is 1. A manifest skill that is not locked yet is noted (`run cali sync`) and also makes the exit code 1.
- `cali remove <name>`: back up and remove a skill (`--force` when it has local edits).
- `cali sync`: install every skill in `cali.lock` and lock manifest skills not locked yet (`--frozen` fails instead of changing `cali.lock`). A skill whose upstream folder is invalid (bad `SKILL.md`, a symlink, a file over the size limit), or whose ref or folder is gone upstream before it is locked, is reported as skipped and left as it was; the rest is installed and the exit code is 1.
- `-g`, `--global` (all commands): use the global scope instead of the project.
- `--manifest <path>` (all commands): use this manifest file; `cali.lock` sits next to it. In project scope its folder is the project root; with `-g` skills still install into `~/.claude/skills` and `~/.copilot/skills`.

## Where things live

Targets (ADR 0005):

| Scope | claude | copilot |
|---|---|---|
| Project | `<project>/.claude/skills/<name>` | `<project>/.github/skills/<name>` |
| Global | `~/.claude/skills/<name>` | `~/.copilot/skills/<name>` |

Project scope keeps `<project>/cali.yaml` and `<project>/cali.lock`; global scope keeps `<config>/cali.yaml` and `<config>/cali.lock`. `--manifest <path>` moves both to the given file and its folder.

| Dir | macOS / Linux / WSL | Windows |
|---|---|---|
| `<config>` | `$CALI_CONFIG_DIR`, else `$XDG_CONFIG_HOME/cali`, else `~/.config/cali` | `%APPDATA%\cali` |
| `<data>` | `$CALI_DATA_DIR`, else `$XDG_DATA_HOME/cali`, else `~/.local/share/cali` | `%LOCALAPPDATA%\cali\data` |
| `<cache>` | `$CALI_CACHE_DIR`, else `$XDG_CACHE_HOME/cali`, else `~/.cache/cali` | `%LOCALAPPDATA%\cali\cache` |

`<cache>` holds disposable git mirrors. `<data>` holds the pristine upstream copies (the merge base for updates) and backups taken before any update or remove; deleting it can make skills `base-lost`.

## Known limits

- Copilot target paths for JetBrains are unverified (`docs/adr/0006-copilot-targets.md`).
- Local edits that only change line endings are not reported as edits (comparison is CRLF-normalized).
- `sync` installs whatever a cloned repo's `cali.yaml` names, so only sync repos you trust.

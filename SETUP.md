# Setup

## Install

You need `go` (1.27+) and `git` on PATH.

```sh
git clone https://github.com/rojcarranza/cali.git ~/Dev/projects/cali
cd ~/Dev/projects/cali
make install        # Git Bash without make: sh scripts/install.sh
cali --version
```

`make install` copies `cali` to `~/.local/bin` (`~/bin` on Git Bash) and adds that folder to PATH if needed. Use `BINDIR=/some/dir make install` for another folder. `make uninstall` removes only the binary.

## Log in to GitHub

Private repos such as `rojcarranza/agent-skills` need a token. Cali reads `GITHUB_TOKEN` or `GH_TOKEN`, then falls back to `gh auth token`:

```sh
gh auth login
```

The token goes only to `github.com`, through git's environment.

## First run

```sh
export CALI_DEFAULT_SOURCE=github:rojcarranza/agent-skills   # add to ~/.zshrc or ~/.bashrc
cali sync -g                                                 # answers yes: installs every skill globally
cali list -g
```

Then see [README.md](README.md) for adding, updating and removing skills.

## Where Cali keeps its files

| Dir | macOS / Linux / WSL | Windows | Holds |
|---|---|---|---|
| config | `~/.config/cali` | `%USERPROFILE%\.config\cali` | global `cali.yaml`, `cali.lock` |
| data | `~/.local/share/cali` | `%LOCALAPPDATA%\cali\data` | pristine copies and backups. Do not delete |
| cache | `~/.cache/cali` | `%LOCALAPPDATA%\cali\cache` | git mirrors, safe to delete |

`CALI_CONFIG_DIR`, `CALI_DATA_DIR` and `CALI_CACHE_DIR` override them. So do the `XDG_*` variables, except on Windows.

Git Bash and WSL can share one global `cali.yaml` ([ADR 0009](docs/adr/0009-home-relative-config-dir.md)):

- **Git Bash and PowerShell** run `cali.exe`, whose `~` is `C:\Users\<you>`. Create the global manifest from there first.
- **WSL** uses `/mnt/c/Users/<you>/.config/cali/` (any drive, or `/c/...` with automount `root = /`) when its own `~/.config/cali/cali.yaml` does not exist. If several Windows users have one, Cali uses none of them, and global commands warn you to set `CALI_CONFIG_DIR`.
- Skills are still installed in each home. After `cali update -g` on one side, run `cali sync -g` on the other.
- An older `%APPDATA%\cali\cali.yaml` keeps working until `%USERPROFILE%\.config\cali` exists.

## Work on the code

```sh
go build -o bin/cali ./cmd/cali     # build
go test -v ./...                    # test
go vet ./... && golangci-lint run   # lint (golangci-lint v2.15.0, same as CI)
```

Try a build without touching your real skills. Point `HOME` and the Cali dirs at a scratch folder:

```sh
(
  export GITHUB_TOKEN="$(gh auth token)"
  export HOME="$PWD/tmp/home" CALI_CONFIG_DIR="$PWD/tmp/config" CALI_DATA_DIR="$PWD/tmp/data" CALI_CACHE_DIR="$PWD/tmp/cache"
  mkdir -p "$HOME"
  ./bin/cali add -g github:rojcarranza/agent-skills --skill roj-grill-plan
  ./bin/cali list -g
)
```

Read first: `docs/brief.md`, `docs/superpowers/specs/2026-10-16-cali-design.md`, and `docs/adr/`. A new dependency needs an ADR first. Tests come first; commits follow Conventional Commits with a scope. `graphify-out/` is a committed code graph: run `graphify update .` after code changes.

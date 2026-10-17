# Setup

How to work on Cali locally, and how to install the `cali` binary. For the commands themselves and where Cali keeps its files, see [README.md](README.md).

## Prerequisites

| Tool | Version | Why |
|---|---|---|
| Go | 1.27 or later (`go.mod`) | Build and test |
| git | any recent version on PATH | Cali runs the system `git` for every fetch and merge (ADR 0001) |
| golangci-lint | v2.15.0 | Lint, same version as CI |
| gh | optional | Supplies a GitHub token when `GITHUB_TOKEN` and `GH_TOKEN` are unset |

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

golangci-lint v2.13.2 fails under Go 1.27.2 with `export data version 5 is greater than maximum supported version 4`.

## Work on the project

```sh
git clone https://github.com/rojcarranza/cali.git
cd cali
go build -o bin/cali ./cmd/cali     # build
go test -v ./...                    # test
go vet ./... && golangci-lint run   # lint
go run ./cmd/cali --version         # run, prints "cali dev"
```

These are the same commands as `CLAUDE.md`. CI (`.github/workflows/ci.yml`) runs the vet, lint and test steps on macOS, Windows and Ubuntu.

`bin/` is build output. Keep it out of git with `.git/info/exclude` rather than a `.gitignore` entry.

### Try it without touching your real skills

Point Cali at throwaway directories. The tests use the same variables:

```sh
export CALI_CONFIG_DIR="$PWD/tmp/config" CALI_DATA_DIR="$PWD/tmp/data" CALI_CACHE_DIR="$PWD/tmp/cache"
mkdir -p tmp/project && cd tmp/project
../../bin/cali init
../../bin/cali add github:<owner>/<repo>//<path/to/skill>
../../bin/cali status
```

Project scope writes into the current repo's `.claude/skills` and `.github/skills`. Global scope (`-g`) writes into your home directory, so use it only when you mean it.

### Where to read first

- `docs/brief.md`: the problem, success metrics and non-goals.
- `docs/superpowers/specs/2026-10-16-cali-design.md`: the design. It is the binding reference.
- `docs/adr/`: one decision per file. A new dependency, datastore or service needs a new ADR first.
- `internal/`: one package per job. Start with `source`, `lock`, `manifest` and `skillfs`; `install`, `status` and `update` build on them.

### Conventions

- Tests first: write the failing test, see it fail, then make it pass.
- Commits follow Conventional Commits with a scope, for example `fix(sync): keep the lock entry when only the ref changed`.
- Never edit `.env*` files.

## Install

From a clone of the repo, on macOS, Linux, WSL or Windows Git Bash:

```sh
make install                # or, where make is missing (Git Bash): sh scripts/install.sh
```

The script detects the OS and CPU, builds `cali` for it, copies it to `~/.local/bin` (`~/bin` on Git Bash), and runs `cali --version`. It needs `go` and `git` on PATH. If the directory is not on PATH it adds it to `~/.zshrc` or `~/.bashrc`; when that file is a symlink (a dotfiles repo) it prints the line to add instead of editing it. On WSL it installs for the Linux side only, so run it again from Git Bash for Windows.

- Somewhere else: `BINDIR=/some/dir make install`
- Remove: `make uninstall` (deletes the binary only; skills and Cali's dirs stay)

The manual steps below do the same by hand.

### macOS

```sh
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o bin/cali ./cmd/cali
mkdir -p ~/.local/bin && cp bin/cali ~/.local/bin/
```

Add `~/.local/bin` to PATH in `~/.zshrc` if it is not there yet: `export PATH="$HOME/.local/bin:$PATH"`.

### Windows (Git Bash, no admin rights)

```sh
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/cali.exe ./cmd/cali
mkdir -p ~/bin && cp bin/cali.exe ~/bin/
```

If `cali` is not found afterwards, add `export PATH="$HOME/bin:$PATH"` to `~/.bashrc` (`~/bin` is `%USERPROFILE%\bin`). Cali uses `%USERPROFILE%`, `%APPDATA%` and `%LOCALAPPDATA%`, so Git Bash and PowerShell see the same skills.

### WSL (Ubuntu)

Build inside WSL, not on the Windows side:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/cali ./cmd/cali
mkdir -p ~/.local/bin && cp bin/cali ~/.local/bin/
```

WSL is a separate install. It manages the Linux home directory only and never writes to the Windows profile.

### With `go install`

Once the code is on `main`:

```sh
go install github.com/rojcarranza/cali/cmd/cali@latest
```

This puts `cali` in `$(go env GOPATH)/bin`, which must be on PATH.

### Check the install

```sh
cali --version          # cali dev
git --version           # Cali needs git on PATH
```

For a private repository or higher rate limits, set `GITHUB_TOKEN` or `GH_TOKEN`, or log in with `gh auth login`. Cali sends the token only to `github.com`, through git's environment, never on the command line.

### First use

New machine, skills already listed in a manifest:

```sh
cali sync -g
cali status -g
```

Moving from APM:

```sh
cali import ~/Dev/projects/agent-skills/apm.yml --local-source github:rojcarranza/agent-skills -g
cali sync -g
cali status -g
```

### Uninstall

Delete the binary. Cali's own state is in `<config>`, `<data>` and `<cache>` (see the README table). Deleting `<data>` also deletes your backups. Installed skill folders stay where they are; remove them with `cali remove <name>` first if you want them gone.

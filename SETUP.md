# Setup

Install the `cali` binary, then work on the code. For how to use it, see [README.md](README.md).

## Install

You need `go` (1.27+, see `go.mod`) and `git` on PATH. Cali runs the system `git` for every fetch (ADR 0001).

```sh
git clone https://github.com/rojcarranza/cali.git ~/Dev/projects/cali
cd ~/Dev/projects/cali
make install              # Git Bash without make: sh scripts/install.sh
cali --version            # cali dev
```

`make install` builds `cali` for your OS and CPU and copies it to `~/.local/bin` (`~/bin` on Git Bash). If that folder is not on PATH, it adds it to `~/.zshrc` or `~/.bashrc`. When that file is a symlink into a dotfiles repo, it prints the line for you to add instead.

- Another folder: `BINDIR=/some/dir make install`
- Uninstall: `make uninstall`. It deletes only the binary. Skills and Cali's folders stay.
- WSL installs for the Linux side only. Run it again from Git Bash to install for Windows.

### Log in to GitHub

`rojcarranza/agent-skills` is private. Cali uses `GITHUB_TOKEN` or `GH_TOKEN`, and falls back to `gh auth token`:

```sh
gh auth login
```

Cali sends the token only to `github.com`, through git's environment, never on the command line. With no token and no git credential helper that knows github.com, a private repo fails with `fatal: could not read Username for 'https://github.com': terminal prompts disabled`.

### First run

Install your skills from the agent-skills clone. Its `install.py` runs `cali sync -g --manifest <clone>/cali.yaml`:

```sh
cd ~/Dev/projects/agent-skills && python3 install.py
cali status -g --manifest ~/Dev/projects/agent-skills/cali.yaml
```

The README walks through the rest: [Example: your agent-skills on every machine](README.md#example-your-agent-skills-on-every-machine).

### Manual build, per platform

`make install` does this for you. Build by hand only if `make` and `sh` are both missing:

```sh
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -o bin/cali     ./cmd/cali   # macOS, Apple silicon
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -o bin/cali     ./cmd/cali   # Linux, WSL (build inside WSL)
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o bin/cali.exe ./cmd/cali   # Windows, Git Bash
```

Copy the binary to a folder on PATH: `~/.local/bin` on macOS and Linux, `~/bin` (`%USERPROFILE%\bin`) on Git Bash. On Windows, Cali reads `%USERPROFILE%`, `%APPDATA%` and `%LOCALAPPDATA%`, so Git Bash and PowerShell see the same skills.

Once the code is on `main`, `go install github.com/rojcarranza/cali/cmd/cali@latest` also works. It puts `cali` in `$(go env GOPATH)/bin`.

## Work on the project

```sh
go build -o bin/cali ./cmd/cali     # build
go test -v ./...                    # test
go vet ./... && golangci-lint run   # lint
go run ./cmd/cali --version         # run
```

These are the commands from `CLAUDE.md`. CI (`.github/workflows/ci.yml`) runs vet, lint and test on macOS, Windows and Ubuntu. Install the same lint version as CI:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
```

v2.13.2 fails under Go 1.27.2 with `export data version 5 is greater than maximum supported version 4`.

`bin/` is build output. Keep it out of git with `.git/info/exclude`, not `.gitignore`.

`graphify-out/` holds a committed knowledge graph of the code (see the graphify section of `CLAUDE.md`). To query or refresh it, install graphify:

```sh
uv tool install graphifyy   # or: pipx install graphifyy
graphify update .           # re-extract changed code, no API key needed
```

Commit only `graph.json`, `graph.html`, `GRAPH_REPORT.md`, `manifest.json` and `.graphify_labels.json`. The other files in `graphify-out/` hold paths for one machine.

### Try a build without touching your real skills

Point every Cali folder, and `HOME`, at a scratch folder. Then even `-g` writes there. The parentheses run it in a subshell, so your own shell keeps its real `HOME`. The token is read first, because `gh` finds its login under the real `HOME`:

```sh
(
  export GITHUB_TOKEN="$(gh auth token)"
  export HOME="$PWD/tmp/home" CALI_CONFIG_DIR="$PWD/tmp/config" CALI_DATA_DIR="$PWD/tmp/data" CALI_CACHE_DIR="$PWD/tmp/cache"
  mkdir -p "$HOME"
  ./bin/cali add -g github:rojcarranza/agent-skills --skill roj-grill-plan
  ./bin/cali status -g
  ls tmp/home/.claude/skills
)
```

### Where to read first

- `docs/brief.md`: the problem, success metrics and non-goals.
- `docs/superpowers/specs/2026-10-16-cali-design.md`: the design, and the binding reference.
- `docs/adr/`: one decision per file. A new dependency, datastore or service needs a new ADR first.
- `internal/`: one package per job. Start with `source`, `lock`, `manifest` and `skillfs`. `install`, `status` and `update` build on them.

### Conventions

- Tests first: write the failing test, watch it fail, then make it pass.
- Commits follow Conventional Commits with a scope, for example `fix(sync): keep the lock entry when only the ref changed`.
- Never edit `.env*` files.

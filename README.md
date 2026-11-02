# Cali

Cali installs agent skills from git repos into Claude Code and GitHub Copilot, pins each one to a commit in `cali.lock`, and updates them later without losing your local edits. Install it first: [SETUP.md](SETUP.md).

Skills land where the IDEs look for them, so once installed you can run them in Claude Code or Copilot:

| Scope | Claude Code | Copilot | `cali.yaml` / `cali.lock` |
|---|---|---|---|
| Project (default) | `.claude/skills/<name>` | `.github/skills/<name>` | project root |
| Global (`-g`) | `~/.claude/skills/<name>` | `~/.copilot/skills/<name>` | `~/.config/cali/` |

## Add skills

```sh
cali add github:rojcarranza/agent-skills                                # every skill in the repo
cali add github:rojcarranza/agent-skills --skill roj-explain-logic      # one skill
cali add github.com/addyosmani/agent-skills --skill code-simplification # someone else's repo
cali add -g github:rojcarranza/agent-skills                             # global: every project
```

A source is `github:<owner>/<repo>`, `github.com/<owner>/<repo>` or an `https://` URL, including a Bitbucket Data Center clone address such as `https://bitbucket.example.com/scm/PROJ/repo.git` (or `scm/~user/repo` for a personal repository). You can also add `//<folder>` to name one skill folder, and `@<ref>` to pin a branch, tag or commit: `github:obra/superpowers//skills/brainstorming@v1.2.0`.

If the repository is a Claude plugin or marketplace (it has `.claude-plugin/plugin.json` or `marketplace.json`), Cali installs only the skills the plugin names, so a repo that keeps a second copy of each skill still installs cleanly.

`--target claude` or `--target copilot` installs for one IDE only.

## Update skills

```sh
cali update                                                          # dry run for every skill
cali update github:rojcarranza/agent-skills --apply                  # every skill from that repo
cali update github:rojcarranza/agent-skills --skill roj-explain-logic --apply
```

Without `--apply`, `update` shows only what would change. If you edited a skill, Cali merges upstream into your edit. When both sides changed the same lines, it stops and asks. Pick `--keep-mine` or `--take-upstream`, or decide per file with `--resolve SKILL.md=ours|theirs|merge`.

## See what you have

```sh
cali list                  # project skills
cali list -g               # global skills
cali list -a               # both, with a SCOPE column
cali find acquire          # search the source repos of project and global skills
cali show roj-explain-logic  # source, commit and install paths (falls back to global)
cali status                # ok, modified (you edited it), or a newer upstream commit
```

## Import the skills your Claude uses

```bash
cali import claude                                   # list skills of your installed, switched-on Claude plugins
cali import claude --manifest ~/agent-skills/cali.yaml -g --skill brainstorming
cali import claude --all
```

Writes each skill to `cali.yaml` pinned to the commit Claude installed, and installs nothing; run `cali sync` after, on this or another computer. Skills already in `cali.yaml` are kept. Without `--skill` or `--all` it asks on a terminal and otherwise only lists.

## Remove a skill

```sh
cali remove acquire-codebase-knowledge
```

The skill is backed up, deleted, and dropped from `cali.yaml`. Cali prints the backup path and the `cali add …` command that restores it. Add `--force` if you edited the skill.

## Sync from `cali.yaml`

Commit `cali.yaml` and `cali.lock`. On another machine or a fresh clone:

```sh
cali sync          # install everything in cali.yaml at the commits in cali.lock
cali sync -g       # install this project's cali.yaml into your home folder, for every project
cali sync --frozen # CI: fail instead of changing cali.lock
cali sync --prune  # also delete installed skills you just dropped from cali.yaml (unmodified only, backed up first); plain sync forgets them, so prune on the same run
```

`cali sync -g` in a project is short for `cali sync -g --manifest cali.yaml`. Only `sync` makes that switch, so pass `--manifest cali.yaml` to later `-g` commands on those skills, for example `cali update -g --manifest cali.yaml --apply`.

**Default global manifest.** To make plain `-g` commands (`list -g`, `find`, `show`, `status -g`, `update -g`, `remove -g`) use that manifest, set its absolute path once in your shell profile:

```sh
export CALI_MANIFEST="$HOME/path/to/agent-skills/cali.yaml"   # your clone
```

`--manifest` still wins over `CALI_MANIFEST`, and `sync -g` in a project still uses that project's `cali.yaml`. Project scope ignores `CALI_MANIFEST`.

**First run.** If there is no `cali.yaml` yet, `sync` offers to install a default skill repo. Set it once in your shell profile:

```sh
export CALI_DEFAULT_SOURCE=github:rojcarranza/agent-skills
```

`cali sync -g` then asks `Install every skill from github:rojcarranza/agent-skills?`. Without a terminal it only prints the `cali add` command to run.

## Commands

| Command | Does | Flags |
|---|---|---|
| `cali add <source>` | Install skills and record them | `--skill`, `--target`, `--adopt` |
| `cali update [source\|names]` | Dry run; `--apply` writes | `--skill`, `--keep-mine`, `--take-upstream`, `--resolve`, `--json`, `--target` |
| `cali list` | Installed skills, and whether each is at its latest upstream version | `-a`, `--json`, `--offline`, `--target` |
| `cali import claude` | Write Claude plugin skills into `cali.yaml`, pinned | `--skill`, `--all`, `--claude-dir` |
| `cali find <query>` | Search source repos | `--offline`, `--json` |
| `cali show <name>` | One skill's source, commit, paths | `--json` |
| `cali status` | Edits, newer commits, missing skills | `--exit-code`, `--offline`, `--json`, `--target` |
| `cali remove <name>` | Back up and remove | `--force`, `--target` |
| `cali doctor` | Folders each app reads; duplicates, leftovers, skipped names, Claude-only skills, unmet `cali-env` and `cali-tools` needs (`cali-tools: python3|python` accepts either) | `--exit-code`, `--json` |
| `cali sync` | Install from `cali.yaml` / `cali.lock` | `--frozen`, `--prune`, `--target` |
| `cali init` | Create an empty `cali.yaml` | |

`--target claude|copilot` on sync, update, status, list and remove acts on that target only (`cali sync -g --target copilot` installs for Copilot only). `remove --target` keeps the skill in `cali.yaml` for its other targets.

`--windows-home` (with `-g`, in WSL) installs into your Windows home, where Windows editors read skills: `cali sync -g --windows-home --target copilot`. Cali finds the one Windows profile, or use `CALI_WINDOWS_HOME`. Git Bash already uses the Windows home (`docs/adr/0010-wsl-installs-into-windows-home.md`).

Every command takes `-g` (global scope) and `--manifest <path>` (use that `cali.yaml`; the lock sits next to it). In global scope, `$CALI_MANIFEST` is the default for `--manifest`.

## Agent mode

For AI agents that run cali in a terminal (Copilot Chat, Claude Code). Pass `--agent` or set `CALI_AGENT=1`:

- Cali never prompts. `cali sync` with no `cali.yaml` prints the `cali add` command instead of asking.
- A usage error (unknown flag or command, wrong arguments) exits 2; other failures exit 1.
- `cali list` and `cali status` open with a total (`27 skills: 25 ok, 2 outdated`, or `0 skills`) and end with a `next:` line naming the command to run.

The repo ships a skill that teaches an agent this: `cali add github:rojcarranza/cali --skill cali -g`.

## Good to know

- `sync` installs whatever a repo's `cali.yaml` names. Sync only repos you trust.
- A skill in the same repo as `cali.yaml` is written `.//skills/<name>`. It is read at the checked-out commit and never fetched.
- Private repos need `gh auth login` or `GITHUB_TOKEN`. See [SETUP.md](SETUP.md).
- `find` shows a one-line ASCII loader on any terminal of known width except `TERM=dumb`. `export CALI_LOADER=princess` brings back the walking ice princess on a color terminal; elsewhere the plain loader stands in.
- Copilot paths for JetBrains are unverified (`docs/adr/0006-copilot-targets.md`). Run `cali doctor` to see the folders each app reads and any duplicate, leftover, skipped or Claude-only skill.

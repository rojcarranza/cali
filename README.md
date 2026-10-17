# Cali

Cali installs agent skills from git repos into Claude Code (`.claude/skills`) and GitHub Copilot (`.github/skills`, `~/.copilot/skills`). It records the source and commit of every skill in `cali.lock`, updates them later, and merges upstream changes into your local edits instead of overwriting them. It never calls an LLM.

Install it first: [SETUP.md](SETUP.md).

## A skill source, written once

Every `add` takes a spec, `<repo>//<folder>@<ref>`:

```
github:rojcarranza/agent-skills//skills/roj-code-review-pr@main
```

- `<repo>` is `github:<owner>/<repo>` or a full `https://` URL.
- `//<folder>` is optional. Without it, use `--skill <name>` or `--all`.
- `@<ref>` is optional: a branch, tag or commit SHA. Without it Cali follows the default branch.

## Example: your agent-skills on every machine

agent-skills commits the skill list as `cali.yaml`, with `cali.lock` next to it pinning each skill to a commit. That repo file is the only manifest. Every command points at it with `-g --manifest`:

- `-g` is where skills go: your home folder (`~/.claude/skills`, `~/.copilot/skills`), so they work in every project.
- `--manifest` is which list: the repo's `cali.yaml`, not the default global one.

**1. Install (new machine or after a pull).** The repo is private, so log in to `gh` once. `install.py` runs `cali sync -g --manifest <clone>/cali.yaml` for you:

```sh
gh auth login
git clone https://github.com/rojcarranza/agent-skills.git ~/Dev/projects/agent-skills
cd ~/Dev/projects/agent-skills
python3 install.py
```

**2. Check what you have.** Run the rest from the agent-skills clone:

```sh
cali list   -g --manifest cali.yaml    # NAME, SOURCE, COMMIT, TARGETS
cali status -g --manifest cali.yaml    # ok, modified (you edited it), or a newer upstream commit
```

Leave out `--manifest` and `cali status -g` reads the default global `cali.yaml` in `<config>` instead, so the repo's skills show as `unmanaged`.

**3. After you push a change to a skill, pull it in:**

```sh
cali update -g --manifest cali.yaml            # dry run: shows what would change
cali update -g --manifest cali.yaml --apply    # writes it, and moves the commit in cali.lock
git commit -am "chore(skills): bump to latest"
```

If you edited an installed skill, Cali merges upstream into your edit. When both changed the same lines it stops and reports a conflict. Keep your version (`--keep-mine`), take upstream (`--take-upstream`), or decide per file for one named skill: `cali update -g --manifest cali.yaml roj-grill-plan --apply --resolve SKILL.md=ours|theirs|merge`.

**4. Add a skill.** `add` installs it and writes it into the repo's `cali.yaml` and `cali.lock`. Comments in the file are kept. Pin community skills to a commit:

```sh
cali add -g --manifest cali.yaml github:rojcarranza/agent-skills//skills/roj-grill-plan
cali add -g --manifest cali.yaml github:github/awesome-copilot//skills/code-tour@7cce7cfb4b61196c36d7e8eb8475ae84b356b126
git add cali.yaml cali.lock && git commit -m "feat(skills): add code-tour"
```

You can also edit `cali.yaml` by hand and run `python3 install.py` (or `cali sync -g --manifest cali.yaml`). An entry looks like this:

```yaml
  caveman:
    source: https://github.com/juliusbrussee/caveman//skills/caveman
    ref: 2e08b9177c07bb7249a8a2d1a6758e5db281d002
    targets: [copilot]      # this skill only; default is the top-level targets
```

**5. Remove one.** It is backed up first. Add `--force` if you edited it:

```sh
cali remove -g --manifest cali.yaml roj-interview-prep
```

If a folder that Cali did not install is already in place and differs from upstream, `add` stops with `exists, is not managed by Cali and differs from upstream; pass --adopt`. Re-run with `--adopt` to keep that copy and manage it from now on.

## Example: one skill in one project

Leave out `-g` and `--manifest`, and Cali works on the current project: `cali.yaml` sits in the project root, and skills land in the repo, so they are committed with it:

```sh
cd ~/Dev/projects/worship-lineup
cali init
cali add github:rojcarranza/agent-skills//skills/roj-grill-plan                          # .claude/skills and .github/skills
cali add github:rojcarranza/agent-skills --skill roj-explain-logic --target claude       # Claude only
git add cali.yaml cali.lock .claude/skills .github/skills
```

Anyone who clones the project runs `cali sync` to get the same commits.

## Commands

| Command | What it does | Useful flags |
|---|---|---|
| `cali init` | Create `cali.yaml` in the project | |
| `cali add <spec>` | Install a skill and record it | `--skill <name>`, `--all`, `--target claude\|copilot`, `--adopt` |
| `cali list` | List installed skills | `--json` |
| `cali show <name>` | Source, commit and install paths of one skill | `--json` |
| `cali find <query>` | Search the skills in your sources and installed skills | `--offline`, `--json` |
| `cali status` | Local edits, newer upstream commits, missing skills | `--exit-code`, `--offline`, `--json` |
| `cali update [names...]` | Dry run of an update; `--apply` writes it | `--keep-mine`, `--take-upstream`, `--resolve <file>=ours\|theirs\|merge`, `--accept-recommended` |
| `cali remove <name>` | Back up, delete, and drop it from `cali.yaml` | `--force` |
| `cali sync` | Install everything in `cali.yaml` / `cali.lock` | `--frozen` (fail instead of changing `cali.lock`, for CI) |

On every command: `-g` uses your home folder instead of the project, and `--manifest <path>` uses that `cali.yaml`, with `cali.lock` next to it.

`update` and `sync` skip a skill whose ref or folder is gone upstream, or whose upstream folder is invalid. They report it, finish the rest, and exit 1. `update` also exits 1 when `cali.yaml` names a skill that is not in `cali.lock` yet, and tells you to `run cali sync`.

## Where things live

| Scope | Claude | Copilot | `cali.yaml` and `cali.lock` |
|---|---|---|---|
| Project | `<project>/.claude/skills/<name>` | `<project>/.github/skills/<name>` | `<project>/` |
| Global (`-g`) | `~/.claude/skills/<name>` | `~/.copilot/skills/<name>` | `<config>/`, or the `--manifest` file's folder |

| Dir | macOS / Linux / WSL | Windows | Holds |
|---|---|---|---|
| `<config>` | `$CALI_CONFIG_DIR`, else `$XDG_CONFIG_HOME/cali`, else `~/.config/cali` | `%APPDATA%\cali` | global `cali.yaml`, `cali.lock` |
| `<data>` | `$CALI_DATA_DIR`, else `$XDG_DATA_HOME/cali`, else `~/.local/share/cali` | `%LOCALAPPDATA%\cali\data` | pristine upstream copies (the merge base) and backups |
| `<cache>` | `$CALI_CACHE_DIR`, else `$XDG_CACHE_HOME/cali`, else `~/.cache/cali` | `%LOCALAPPDATA%\cali\cache` | git mirrors, safe to delete |

Deleting `<data>` loses your backups and can mark skills `base-lost`.

## Known limits

- Copilot target paths for JetBrains are unverified (`docs/adr/0006-copilot-targets.md`).
- An edit that only changes line endings is not reported (comparison is CRLF-normalized).
- `sync` installs whatever a cloned repo's `cali.yaml` names, so only sync repos you trust.

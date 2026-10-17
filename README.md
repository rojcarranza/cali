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

`rojcarranza/agent-skills` is private, so log in once and Cali takes the token from `gh`:

```sh
gh auth login
```

**1. Install all your skills, globally, for Claude and Copilot:**

```sh
cali add -g github:rojcarranza/agent-skills --all
```

The two flags answer different questions. `-g` is where: your home folder, so the skills work in every project. `--all` is which: every skill in the repo. Leave out `-g` to install into the current project. Use `--skill <name>` instead of `--all` to install one skill.

This creates the global `cali.yaml`, then copies each skill to `~/.claude/skills/<name>` and `~/.copilot/skills/<name>`. If `install.py` already copied a skill there and your copy differs, Cali stops with `exists, is not managed by Cali and differs from upstream; pass --adopt`. Re-run with `--adopt` to keep your copy and manage it from now on.

**2. Add a community skill, pinned to a commit:**

```sh
cali add -g github:github/awesome-copilot//skills/code-tour@7cce7cfb4b61196c36d7e8eb8475ae84b356b126
```

**3. Check what you have:**

```sh
cali list -g      # NAME, SOURCE, COMMIT, TARGETS
cali status -g    # ok, modified (you edited it), or a newer upstream commit
```

**4. After you push a change to agent-skills, pull it in:**

```sh
cali update -g            # dry run: shows what would change
cali update -g --apply    # writes it
```

If you edited a skill locally, Cali merges upstream into your edit. When both changed the same lines it stops and reports a conflict. Keep your version (`--keep-mine`), take upstream (`--take-upstream`), or decide per file for one named skill: `cali update -g roj-grill-plan --apply --resolve SKILL.md=ours|theirs|merge`.

**5. Remove one:**

```sh
cali remove -g roj-interview-prep    # backs it up first; --force if you edited it
```

**6. New machine:** copy the global `cali.yaml` and `cali.lock` (paths under [Where things live](#where-things-live)), then:

```sh
cali sync -g
```

## Example: one skill in one project

Run this from the project root. Skills land in the repo, so they are committed with the project:

```sh
cd ~/Dev/projects/worship-lineup
cali init
cali add github:rojcarranza/agent-skills//skills/roj-grill-plan                          # .claude/skills and .github/skills
cali add github:rojcarranza/agent-skills --skill roj-explain-logic --target claude       # Claude only
git add cali.yaml cali.lock .claude/skills .github/skills
```

A teammate who clones the repo runs `cali sync` to get the same commits.

## Example: a manifest kept in agent-skills

You can keep the skill list in a `cali.yaml` inside agent-skills itself and point Cali at it:

```yaml
# ~/Dev/projects/agent-skills/cali.yaml
version: 1
targets: [claude, copilot]
skills:
  roj-code-review-pr:
    source: github:rojcarranza/agent-skills//skills/roj-code-review-pr
  code-tour:
    source: github:github/awesome-copilot//skills/code-tour
    ref: 7cce7cfb4b61196c36d7e8eb8475ae84b356b126
  caveman:
    source: https://github.com/juliusbrussee/caveman//skills/caveman
    ref: 2e08b9177c07bb7249a8a2d1a6758e5db281d002
    targets: [copilot]      # this skill only
```

```sh
cd ~/Dev/projects/agent-skills
cali sync -g --manifest cali.yaml        # install, and write cali.lock next to it
cali status -g --manifest cali.yaml
cali update -g --manifest cali.yaml --apply
```

Pass `--manifest` on every command. Without it, `cali status -g` reads the default global `cali.yaml` and lists these skills as `unmanaged`.

## Commands

| Command | What it does | Useful flags |
|---|---|---|
| `cali init` | Create `cali.yaml` in the project (`add -g` creates the global one itself) | |
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
| Global (`-g`) | `~/.claude/skills/<name>` | `~/.copilot/skills/<name>` | `<config>/` |

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

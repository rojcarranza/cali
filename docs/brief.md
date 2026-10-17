# Cali Brief

Status: approved (2026-10-16)
Repo: https://github.com/rojcarranza/cali

## Problem
My agent skills come from my own repo (`rojcarranza/agent-skills`) and from several upstream repos, and they reach my machines through four install routes (APM, `install.py`, `npx skills`, a Claude Code plugin). Updating an upstream skill I have customized overwrites my changes, and nothing tells me which skills are outdated, edited or missing a skill they rely on.

## Target user and main job-to-be-done
- User: me (Roj), on a personal Mac and a work Windows machine (Git Bash, VDI) plus WSL, using Claude Code and GitHub Copilot.
- Job: "Get the right skill versions into this project or agent, keep them current, and never lose my own edits."
- Skills cover work and personal life (code review, DevOps, finances, faith). Cali treats them all the same.

## Pitch
Cali is one Go binary that installs, pins, updates and syncs agent skills from any git source. It records where every skill came from and merges upstream changes into my edits instead of overwriting them. It replaces APM, `install.py`, `npx skills` and the plugin route with one command and one lockfile.

## Success metrics
- `install.py` and APM are retired from my machines by the end of the MVP (target: 2026-11-27).
- Fresh machine to all skills installed: `cali sync` finishes in under 2 minutes (excluding git clone time over slow networks).
- Zero local edits lost on `cali update` (every conflict stops the update; covered by tests).
- `cali status` correctly flags 100% of: local edits, newer upstream commits, missing declared dependencies (covered by tests).
- Same `cali.lock` produces byte-identical skill folders on macOS, Windows and WSL.

## MVP scope
- Commands: `init`, `add`, `list`, `show`, `find`, `status`, `update` (dry run by default, `--apply` merges), `remove`, `sync`, ~~`import apm.yml`~~ (removed 2026-10-17, issue #13: APM is retired, so `cali import` was dropped).
- Sources: any git repo (GitHub first), pinned to an exact commit in `cali.lock`; project scope and global scope.
- Targets: Claude Code and GitHub Copilot skill folders on macOS, Windows (Git Bash) and WSL. Files are copied, not symlinked.
- Safe updates: Cali stores the pristine upstream copy of each installed version and runs a 3-way merge on update; conflicts stop and show the conflicting lines.
- Dependencies: declared dependencies are checked and reported as warnings in `status`; never installed automatically.
- Output: `sbx`-style styled tables, status badges, spinners and prompts; plain text and `--json` when piped.

## Non-goals
1. Not an AI agent. Cali never calls an LLM.
2. No `load` (streaming skills into agent context) and no `run` (executing skill scripts) in the MVP.
3. No automatic dependency resolution (versions, cycles, transitive installs). Warnings only.
4. No full-screen interactive dashboard in the MVP (planned for v1.1).
5. No hosted registry, website or public catalog service.
6. No GUI, VS Code extension or JetBrains plugin.
7. No managing MCP servers, agent instructions files or non-skill config (APM did this; Cali does not, for now). MCP servers move to a small standalone setup script in `agent-skills` when APM is retired.
8. No team or multi-user features (shared registries, permissions, publishing workflows).
9. No support for agents beyond Claude Code and GitHub Copilot in the MVP.
10. No skill authoring or evaluation tooling (writing, linting or testing SKILL.md content).

## Constraints
- Budget: EUR 0. Personal project, open-source tools only.
- Language: Go, single static binary per OS.
- Platforms: macOS, Windows (Git Bash, locked-down VDI, possibly no admin rights), WSL Ubuntu.
- Network: work machine may block some hosts; GitHub over HTTPS must be enough.
- Timeline: about 6 weeks at 5 to 6 hours per week (2026-10-19 to 2026-11-27).
  - Week 2: walking skeleton (one skill from one git repo into one agent folder, CI green).
  - Week 4: `add`, `sync`, `status`, `list`.
  - Week 6: `update` with 3-way merge, ~~`import apm.yml`~~ (removed 2026-10-17, issue #13), retire `install.py`.
- Compliance: no secrets in config or lockfile; tokens come from the environment or `gh auth token`.

## Open questions
- How do skills declare dependencies? (No field exists in the Agent Skills spec; options: a `requires:` frontmatter key, or a field in `cali.yaml`.)
- What are the exact Copilot skill folder paths for VS Code and JetBrains on each OS?
- Should a skill be installable from a subfolder of a monorepo (e.g. `obra/superpowers/skills/x`) with history per subfolder, or per repo?
- Where does Cali keep pristine copies and cache (`~/.cache/cali`, `~/.local/share/cali`, `%LOCALAPPDATA%`)?
- What happens to skills APM installed that are not in `apm.yml` (cleanup on import)? (Moot since 2026-10-17: `import` was removed, issue #13.)
- Name clashes: two sources with the same skill name.

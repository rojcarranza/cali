---
name: cali
description: Install, check and update AI agent skills with the cali CLI. Use when asked to install, sync, list, update or remove skills, or to see which skills are outdated or missing.
---

# Using cali

Cali installs agent skills from git repos into Claude Code and GitHub Copilot, and pins them in `cali.lock`.

Always run it in agent mode, so it never waits for input:

```sh
export CALI_AGENT=1        # or pass --agent on every command
```

In agent mode:
- Cali never prompts. With no `cali.yaml`, `cali sync` prints the `cali add` command to run instead of asking.
- A usage error (unknown flag, unknown command, wrong arguments) exits 2. Any other failure exits 1.
- `cali list` and `cali status` start with a total, such as `27 skills: 25 ok, 2 outdated`, and `0 skills` when nothing is installed.
- A `next:` line at the end names the command to run next. Run it, or tell the user why not.

## Common tasks

| Task | Command |
|---|---|
| See what is installed and its state | `cali status -g` (home folder) or `cali status` (project) |
| Install every skill in a repo | `cali add github:<owner>/<repo> [-g]` |
| Install one skill | `cali add github:<owner>/<repo> --skill <name> [-g]` |
| Install what `cali.yaml` lists | `cali sync [-g]` |
| Preview updates | `cali update [-g]` |
| Apply updates | `cali update --apply [-g]` |
| Remove a skill | `cali remove <name> [-g]` |

Add `--json` for machine-readable output. Add `-g` to use the home folder instead of the project.

## Rules

- Run `cali status` before and after a change, and report the totals line.
- `cali update` is a dry run; only `--apply` writes. If it reports a conflict, stop and ask the user which side to keep (`--keep-mine` or `--take-upstream`).
- Do not use `--force` or `--adopt` unless the user asked for it.
- Do not edit installed skill folders by hand; change the source and run `cali update`.

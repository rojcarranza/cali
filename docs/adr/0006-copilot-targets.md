# ADR 0006: Copilot target paths

Status: accepted — V1 and V2 are answered on each machine by `cali doctor`, not decided here

## Context

Cali writes the `copilot` target to `<project>/.github/skills/<name>/` (project scope) and `~/.copilot/skills/<name>/` (global scope), and the `claude` target to `<project>/.claude/skills/<name>/` and `~/.claude/skills/<name>/` (design spec section 6). Documentation settles the VS Code and Copilot CLI paths, but three points can only be answered by hand on real clients (Task 7, Step 8). JetBrains stays "unsupported" until this ADR records otherwise.

## Questions

- **V1 (JetBrains Copilot skill paths):** where does the JetBrains Copilot plugin read skills from? Install one skill with both targets (`cali add <spec>` with the default `[claude, copilot]`), open the project in a JetBrains IDE with the Copilot plugin, and record which of `.github/skills`, `.claude/skills`, `~/.copilot/skills`, `~/.claude/skills` (or none) it finds the skill in.
- **V2 (duplicate listing in VS Code):** with both targets installed in one project, does VS Code Copilot list the skill twice (once via `.claude/skills`, once via `.github/skills`)? If yes, the fallback in spec section 6 applies (a manifest option such as `targets: [claude]` for VS Code-only users).
- **Windows form of `~/.claude/skills`:** confirm that Claude Code on Windows reads `%USERPROFILE%\.claude\skills\` (the form currently comes from third-party guides, not Claude Code docs).

## Findings

No client was opened for this ADR. What is known:

- **V1 (JetBrains):** the agent-skills guide says the JetBrains Copilot plugin reads `~/.copilot/skills`. `cali doctor` lists only that folder for JetBrains. JetBrains skills are a preview feature, so under Copilot Business an admin must allow them.
- **V2 (duplicates):** VS Code documents reading `.github/skills`, `.claude/skills` and `.agents/skills` (project) and the matching personal folders. A skill installed for both targets sits in two of them. Nobody has confirmed whether VS Code lists it twice.
- **Windows form of `~/.claude/skills`:** still from third-party guides.

## Decision

Cali keeps the two targets of design spec section 6 and does not wait for V1 or V2. `cali doctor` closes the question in practice: it lists the folders each app reads, flags every skill an app can see in more than one folder (V2), and reports JetBrains per the guide (V1). A user who sees a real duplicate in their client sets `targets: [copilot]` in cali.yaml. Doctor also flags names VS Code skips, Claude-only skills (SKILL.md keys `allowed-tools`, `model`, `effort`, `context`, `agent`, `hooks`, `shell`, `paths`), leftovers not in cali.yaml, and unmet needs declared in `metadata.cali-env` and `metadata.cali-tools`.

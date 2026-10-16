# ADR 0006: Copilot target paths

Status: proposed — unverified, awaiting manual check by Roj

## Context

Cali writes the `copilot` target to `<project>/.github/skills/<name>/` (project scope) and `~/.copilot/skills/<name>/` (global scope), and the `claude` target to `<project>/.claude/skills/<name>/` and `~/.claude/skills/<name>/` (design spec section 6). Documentation settles the VS Code and Copilot CLI paths, but three points can only be answered by hand on real clients (Task 7, Step 8). JetBrains stays "unsupported" until this ADR records otherwise.

## Questions

- **V1 (JetBrains Copilot skill paths):** where does the JetBrains Copilot plugin read skills from? Install one skill with both targets (`cali add <spec>` with the default `[claude, copilot]`), open the project in a JetBrains IDE with the Copilot plugin, and record which of `.github/skills`, `.claude/skills`, `~/.copilot/skills`, `~/.claude/skills` (or none) it finds the skill in.
- **V2 (duplicate listing in VS Code):** with both targets installed in one project, does VS Code Copilot list the skill twice (once via `.claude/skills`, once via `.github/skills`)? If yes, the fallback in spec section 6 applies (a manifest option such as `targets: [claude]` for VS Code-only users).
- **Windows form of `~/.claude/skills`:** confirm that Claude Code on Windows reads `%USERPROFILE%\.claude\skills\` (the form currently comes from third-party guides, not Claude Code docs).

## Findings

## Decision

Pending the findings above.

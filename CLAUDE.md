# Cali

Go CLI for discovering, installing, loading, updating and managing AI agent skills.
Status: pre-build. Phases tracked in `docs/` (brief, design, requirements, architecture, plan).

## Commands
<!-- Filled and verified in Phase 5. Do not guess commands until then. -->
- Build: TBD
- Test: TBD
- Lint: TBD
- Run: TBD

## Rules
- Work only on the current task. One phase or task at a time.
- Tests first (TDD). A task is done only when its verify command passes.
- Follow the ADRs in `docs/adr/`. No new dependency, datastore or service without a new ADR.
- Small commits with clear messages.
- Ask before deleting, overwriting or renaming any existing file.
- Never touch `.env*` files.

## When compacting, keep
- Current phase or task, and its requirement IDs
- Open decisions and anything the user rejected

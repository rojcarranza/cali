# Cali

Go CLI for discovering, installing, loading, updating and managing AI agent skills.
Status: pre-build. Phases tracked in `docs/` (brief, design, requirements, architecture, plan).

## Commands
<!-- Filled and verified in Phase 5. Do not guess commands until then. -->
- Build: `go build -o bin/cali ./cmd/cali`
- Test: `go test -v ./...`
- Lint: `go vet ./... && golangci-lint run`
- Run: `go run ./cmd/cali --version`

## Rules
- Work only on the current task. One phase or task at a time.
- Tests first (TDD). A task is done only when its verify command passes.
- Follow the ADRs in `docs/adr/`. No new dependency, datastore or service without a new ADR.
- Small commits with clear messages.
- Ask before deleting, overwriting or renaming any existing file.
- Never touch `.env*` files.

## Pull requests
- No PR without a GitHub issue or user story. Link it in the PR body (`Closes #<n>` for an issue, or the user story ID). If none exists, create or ask for one before opening the PR.
- Right after opening a PR, run `/code-review` on it. Fix every confirmed finding on the PR branch, re-run the tests, and push before asking for merge.

## When compacting, keep
- Current phase or task, and its requirement IDs
- Open decisions and anything the user rejected

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).

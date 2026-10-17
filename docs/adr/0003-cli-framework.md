# ADR 0003: CLI framework

Status: accepted (2026-10-16)

## Context

Cali has about ten subcommands (`init`, `add`, `list`, `show`, `find`, `status`, `update`, `remove`, `sync`, `import`) with per-command flags, a persistent `-g` flag, `--json`, and shell completion is useful. `main` calls `cli.Execute(args, out, errw) int`, so the framework must work with injected arguments and writers for testing and must not call `os.Exit` itself.

## Decision

Use `github.com/spf13/cobra`, version `v1.10.2` (verified: `go list -m github.com/spf13/cobra@latest` resolves to v1.10.2, published 2025-12-03). It is widely used (kubectl, gh, hugo), MIT-licensed, pure Go, and builds to a single static binary on macOS, Windows and Linux. It supports `SetArgs`, `SetOut` and `SetErr`, which is what `Execute` needs.

Alternatives considered:
- `urfave/cli`: also maintained and fine, but has a weaker subcommand and flag-inheritance model and less ecosystem familiarity.
- `alecthomas/kong`: struct-tag based and compact, but smaller community and less standard help and completion output.
- Standard library `flag`: no subcommand support; would mean hand-rolling dispatch, help and completion.

## Consequences

- One new dependency (plus `spf13/pflag` and `mousetrap` transitively), to be added by the task that builds the command tree.
- The `cli` package owns all cobra usage; other packages stay framework-free.
- Cobra's default error and usage printing is configured in `cli`, not left to defaults, so `--json` and exit codes stay under Cali's control.

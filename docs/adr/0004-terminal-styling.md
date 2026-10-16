# ADR 0004: Terminal styling, spinner and prompts

Status: accepted (2026-10-16)

## Context

Spec section 10: styled tables and spinners on a TTY, plain text and `--json` when piped. `add` also asks the user to pick skills interactively when a repo holds several. This must work on macOS, Linux, WSL, Windows Terminal and Git Bash, and degrade to plain output with no color when stdout is not a TTY or `NO_COLOR` is set.

## Decision

Use the Charm libraries:
- `charm.land/lipgloss/v2`, version `v2.0.6` (verified resolvable with `go list -m charm.land/lipgloss/v2@latest`, published 2026-08-11) for styles and table layout, with color profile detection and downsampling.
- `charm.land/huh/v2`, version `v2.0.3` (verified resolvable, published 2026-03-10) for the interactive select/multi-select prompts and its spinner package.

Both are MIT-licensed, pure Go, actively maintained, and build into a single static binary on all three OSes, including Windows consoles. Cali enables them only when stdout is a TTY; otherwise it prints plain text from its own code path, so piped output and tests never contain escape codes.

Alternatives considered:
- `github.com/charmbracelet/lipgloss` v1.1.0 and `huh` v1.0.0: the old import paths, superseded by the `charm.land` v2 modules; rejected to avoid starting on a legacy major version.
- `fatih/color` plus `briandowling/spinner`-style packages plus `survey`: three small libraries instead of one family; `survey` is archived.
- Hand-written ANSI: cross-platform Windows console handling is the hard part and not worth owning.

## Consequences

- Two new direct dependencies (and Charm's transitive set), to be added by the task that first renders output.
- Plain-text and `--json` output paths are separate from the styled path and covered by tests that never see a TTY.
- Binary size grows modestly; acceptable for a CLI distributed as a single file.

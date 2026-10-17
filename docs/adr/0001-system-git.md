# ADR 0001: Shell out to system git and use git merge-file

Status: accepted (2026-10-16)

## Context

Cali fetches skills from git repos (GitHub first, HTTPS only), reads blobs and tree SHAs at a given commit, and 3-way merges local edits with upstream changes on `update` (design spec sections 4, 5 and 10). It must work on macOS, Linux, WSL and Windows (Git Bash, PowerShell). Git is already installed on every machine where a developer uses these agents. Credential helpers, `gh` and corporate proxy settings already work through the `git` binary.

## Decision

- Cali runs the system `git` binary for all repository work: bare mirror clones and `git fetch` (with `--filter=blob:none`, falling back to a normal fetch), `git cat-file` / `git archive` to materialize files from blobs, and `git rev-parse` for commit and tree SHAs.
- 3-way text merges use `git merge-file -p --diff3 ours base theirs`; exit code greater than 0 means conflict.
- Tokens (`GITHUB_TOKEN`, `GH_TOKEN`, or `gh auth token`) reach git through `GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_n` / `GIT_CONFIG_VALUE_n` environment variables, never on the command line, never in the manifest, lock or logs, and only for `github.com`.
- No Go git library (go-git or similar) and no Go merge library is added.

## Consequences

- No new dependency; the binary stays small and static.
- Credential helpers, SSH config and proxies behave as they do on the command line.
- `git` must be installed and on `PATH`. Cali reports a clear error when it is missing.
- Behavior depends on the installed git version. Cali relies only on long-standing features (`merge-file --diff3`, `cat-file`, `archive`, `GIT_CONFIG_COUNT` from git 2.31).
- Tests that touch git need a `git` binary; CI runners on all three OSes have one.
- Alternative rejected: go-git (pure Go) adds a large dependency, lags on partial-clone and credential-helper support, and its merge story is weaker.

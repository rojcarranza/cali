# ADR 0007: Same-repo sources

Status: accepted (2026-10-18)

## Context

A skills repo that ships its own `cali.yaml` (rojcarranza/agent-skills) had to name its own skills by their GitHub URL. Sync then resolved them against the remote default branch, so a skill added on a branch was skipped as `not found upstream` until it merged, and `cali.lock` could only be bumped in a second change after the merge (cali#17, agent-skills#39 and #40). A `file://` source would not work on another machine, and a lock entry cannot pin the commit that will contain it.

## Decision

- Source spec `.//path` (or `.` for the repo root) names the git repository that holds the scope's manifest. `path` is from that repository's root. It takes no ref, neither `@ref` nor `ref:`.
- Cali reads it in place at the checked-out `HEAD` (`git rev-parse --absolute-git-dir` from the manifest's folder, so linked worktrees work), with no fetch, no mirror in `<cache>` and no transport allowed (`GIT_ALLOW_PROTOCOL` empty, `GIT_NO_LAZY_FETCH=1`). Uncommitted edits are not installed: the lock must describe what CI sees on the same commit.
- Its lock entry is `{"source": ".", "path": ..., "ref": "", "commit": "", "tree": <tree SHA>}`. There is no commit: the commit that will hold the lock does not exist when it is written, and a squash merge drops branch commits. The tree SHA names the content.
- `sync` compares the locked tree with the tree at `HEAD`. A difference is reported as `outdated` (exit 0), and `sync --frozen` fails with `locked tree is behind the repo, run cali update`. `update --apply` moves the tree, as for any other source, and still writes no commit.
- A missing pristine copy is rebuilt from the tree object when the repository still has it, otherwise the skill is `base-lost`.

## Consequences

- Adding a skill to a skills repo is one change: the folder, its `cali.yaml` entry and its `cali.lock` entry, installable before the merge.
- A committed lock that falls behind its own repository fails `sync --frozen`, so CI catches it.
- Cali now reads (never writes) a repository outside `<cache>`: the one holding the manifest. ADR 0005's write boundaries are unchanged.
- No new dependency, datastore or service.

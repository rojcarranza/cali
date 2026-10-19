# ADR 0008: Default global manifest

Status: accepted (2026-10-19)

## Context

ADR 0005 puts the global manifest at `<config>/cali.yaml`, overridable with `--manifest`. Global skills managed by a skills repo's own `cali.yaml` (rojcarranza/agent-skills, installed with `cali sync -g --manifest <clone>/cali.yaml`) are invisible to every other `-g` command unless `--manifest` is repeated: `list -g`, `find`, `show`, `status -g`, `update -g` and `remove -g` read `<config>/cali.yaml` and report those skills as unmanaged (cali#29).

## Decision

- In global scope, when `--manifest` is not given, `$CALI_MANIFEST` names the manifest. The lock sits next to it, as with `--manifest`.
- It must be an absolute path; a relative one is an error, so the result does not depend on the cwd.
- Precedence: `--manifest`, then (for `sync -g` only) the project's `cali.yaml`, then `$CALI_MANIFEST`, then `<config>/cali.yaml`.
- Project scope ignores it. In project scope, `status` reads the global manifest through the same rule, so it sees the same global skills.

## Consequences

- One line in a shell profile makes every global command see the skills a skills repo manages.
- Cali reads and writes that manifest and lock as it does with `--manifest`; ADR 0005's write boundaries already cover a manifest named by the user.
- No new dependency, datastore or service.

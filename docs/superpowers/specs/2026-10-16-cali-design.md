# Cali design

Status: draft for review (2026-10-16)
Input: `docs/brief.md` (approved 2026-10-16). This document resolves every item under "Open questions" in the brief and fixes the formats and algorithms the plan needs. It does not change scope; the brief's non-goals apply unchanged.

Notation: **Decided** = a call made in this document (mine unless marked otherwise). **Verify** = a fact I could not confirm from documentation; a walking-skeleton task must check it before the dependent code is relied on.

## 1. Model

- **Source**: a git repo (GitHub first, HTTPS only).
- **Skill**: a folder containing `SKILL.md`, located at a `path` inside a source (`""` = repo root).
- **Install**: one skill, in one scope (project or global), written to one or more targets.
- **Scope**: project = nearest ancestor directory containing `cali.yaml` (else the git root, created by `cali init`); global = `-g`.
- **Target**: `claude` or `copilot`. A target + scope maps to one skills directory (section 6).
- **Install name**: the skill folder name. It must equal the `name` in `SKILL.md` frontmatter. Copilot silently skips a skill whose `name` does not match its parent directory (VS Code docs), so Cali never renames a folder.

### Source spec syntax (CLI and manifest)

```
github:owner/repo                       whole repo (must contain exactly one skill, or use --skill / --all)
github:owner/repo//skills/brainstorming subfolder of a monorepo
github:owner/repo//skills/x@v1.2.0      branch, tag or full commit
https://host/owner/repo.git//path@ref   explicit URL
```

The canonical URL lowercases the host, and for `github.com` also the owner and repo, because GitHub ignores their case. Other hosts keep the owner and repo as written.

## 2. Files and where they live

| What | Project scope | Global scope |
|---|---|---|
| Manifest `cali.yaml` (committed) | `<project>/cali.yaml` | `<config>/cali.yaml` |
| Lock `cali.lock` (committed) | `<project>/cali.lock` | `<config>/cali.lock` |
| Installed skills | `<project>/.claude/skills/<name>`, `<project>/.github/skills/<name>` | `~/.claude/skills/<name>`, `~/.copilot/skills/<name>` |

Per-user directories (**Decided**), resolved with `os.UserHomeDir()` (on Windows this is `%USERPROFILE%`, not the MSYS `$HOME`, so Git Bash and PowerShell agree):

| Dir | macOS / Linux / WSL | Windows |
|---|---|---|
| `<config>` | `$CALI_CONFIG_DIR`, else `$XDG_CONFIG_HOME/cali`, else `~/.config/cali` | `%APPDATA%\cali` |
| `<data>` (pristine copies, backups; not rebuildable offline) | `$CALI_DATA_DIR`, else `$XDG_DATA_HOME/cali`, else `~/.local/share/cali` | `%LOCALAPPDATA%\cali\data` |
| `<cache>` (git mirrors; always rebuildable) | `$CALI_CACHE_DIR`, else `$XDG_CACHE_HOME/cali`, else `~/.cache/cali` | `%LOCALAPPDATA%\cali\cache` |

macOS deliberately uses the XDG-style paths rather than `~/Library/...`: it matches the dotfiles setup and keeps one rule for Mac, Linux and WSL. `--manifest <path>` overrides the manifest location so the global manifest can live in the dotfiles repo; the lock sits next to it.

Cali never edits `.gitignore`. Whether installed skill folders are committed in a project is the user's choice; `cali.yaml` + `cali.lock` are enough to rebuild them with `cali sync`.

## 3. `cali.yaml` (manifest, hand-editable)

```yaml
version: 1
targets: [claude, copilot]        # default for entries below
sources:                          # optional; repos `cali find` searches without installing
  - github:obra/superpowers
skills:                           # map keyed by install name, so a name can appear only once
  code-review:
    source: github:rojcarranza/agent-skills//skills/code-review
    ref: main                     # branch/tag/commit to track; default = remote default branch
  brainstorming:
    source: github:obra/superpowers//skills/brainstorming
    ref: v5.0.0
    targets: [claude]             # per-skill override
    requires: [writing-plans]     # see section 7
```

Rules: unknown keys are an error (typos must not pass silently); no secrets are ever stored here; `ref` set to a full commit SHA means "pinned, never moves". Cali rewrites the file only for `add`/`remove`/`import`, and preserves comments if the YAML library allows it (**Verify** when the library is chosen; if not, say so in the ADR).

## 4. `cali.lock` (generated, deterministic)

JSON, not YAML: the standard library can write it, so it adds no dependency and its bytes are fully controlled.

```json
{
  "version": 1,
  "skills": {
    "code-review": {
      "source": "https://github.com/rojcarranza/agent-skills",
      "path": "skills/code-review",
      "ref": "main",
      "commit": "9f2c...(40 hex)",
      "tree": "b81e...(40 hex)"
    }
  }
}
```

- `commit`: exact upstream commit. `tree`: the git tree SHA of `path` at that commit. The tree SHA is the identity of the skill content: it is computed by git over the upstream objects, so it is identical on every OS.
- Writer rules (these make "same lock, byte-identical folders" testable): keys sorted, 2-space indent, `\n` line endings, trailing newline, no timestamps, no absolute paths, no per-machine data. Targets and scope are not in the lock; they come from the manifest.
- `source` is normalized to an `https://` URL without credentials.

### Byte-identical installs across OSes

- Files are materialized from git blobs (`git cat-file`/`git archive`), never from a working-tree checkout, so `core.autocrlf` cannot alter them. Bytes written = bytes in the commit.
- Only the executable bit is carried over (ignored on Windows). Timestamps are not part of the contract.
- Symlinks, submodules, paths containing `..`, absolute paths, and Windows-reserved names (`CON`, `NUL`, trailing dot/space, `:`) inside a skill folder make `add`/`sync` fail with the offending path. (Security: nothing in a skill is ever executed by Cali.)
- CI runs a golden test on a macOS/Windows/Ubuntu matrix: one fixture lock, hash of the resulting folder must match. WSL is covered by the Ubuntu job, since a WSL binary is a Linux binary.

## 5. Pristine copies and the 3-way merge

### Storage

- `<cache>/git/<host>/<owner>/<repo>.git`: one bare mirror per source, fetched with `git fetch`. Disposable.
- `<data>/pristine/<tree-sha>/`: plain-file copy of the upstream skill folder for each tree SHA that is installed anywhere on this machine, shared by all scopes and targets. It is under `<data>`, not `<cache>`, because it is the merge base: a force-pushed upstream or a garbage-collected mirror must not destroy it. If it is missing (e.g. fresh machine), `sync` rebuilds it from the lock's `commit`; if the commit is gone upstream, Cali says so and the skill is reported `base-lost` (updates for it need `--take-upstream`).
- A pristine dir is removed only by `remove`, and only when no lock on this machine still references that tree.

### Detecting state

Compare each installed target folder with `pristine/<tree>`:

- Text files (no NUL byte in the first 8000 bytes) are compared after normalizing CRLF to LF, so an editor on Windows that rewrites line endings is not reported as an edit. Binary files are compared byte for byte.
- File set differences (added/removed files) count as edits.

### `update` algorithm

For each selected skill and target, with `base` = pristine at the locked tree, `theirs` = upstream at the new commit (tree of the same `path`), `ours` = installed folder:

1. If `tree(new) == tree(locked)`: nothing to do, even if the repo has newer commits (they did not touch this skill).
2. Plan per path (union of file names):

| ours vs base | theirs vs base | result |
|---|---|---|
| same | any | take theirs (add / modify / delete) |
| changed | same | keep ours |
| changed | changed, ours == theirs | keep |
| changed | changed, different, text | line merge with `git merge-file -p --diff3 ours base theirs`; exit code > 0 = conflict |
| changed | changed, different, binary | conflict |
| deleted | modified (or reverse) | conflict |

   `git merge-file` is used instead of a Go merge library: git is already required for fetching, it needs no new dependency, and its behavior is the well-known one. Files are merged on LF-normalized text and written back with the installed file's existing line-ending style.
3. **Dry run (default)** prints the plan: per file `take-upstream | keep-local | merged | CONFLICT`, and for each conflict the conflicting hunk (ours / base / theirs lines with line numbers). Nothing is written.
4. **`--apply`**: all skills are planned first. If any skill has a conflict, nothing is written for any skill and the exit code is non-zero (brief: every conflict stops the update). `cali update <name> --apply` updates only the named skills, which is how clean skills proceed while a conflict is dealt with.
5. Writes are staged: build the merged folder next to the target (`<name>.cali-new`), copy the current folder to `<data>/backups/<name>/<UTC-timestamp>/`, then swap by rename (rename old to `<name>.cali-old`, new to `<name>`, delete old; this order works on Windows, where renaming over a non-empty directory fails). On any error the old folder is restored. The lock is written last, after all targets succeeded.
6. The new `pristine/<new-tree>` is stored, then the lock gets the new `commit` and `tree`.

### Resolving a conflict (**Decided by Roj**: no conflict markers in files; Cali analyzes and recommends)

Installed files are never left containing `<<<<<<<` markers. A conflict is resolved by an explicit choice per file, and Cali tells you which choice it recommends and why. The analysis is plain deterministic rules (non-goal 1: no LLM), so every recommendation is reproducible and unit-testable.

**1. Pre-resolution (shrinks the conflict list before anything is shown).** A file is not a conflict if, after the rules below, one side's change is empty:
- one side differs from base only in whitespace or line endings: take the other side;
- both sides made the same change: keep it (already in the table above).

**2. Classification.** For each remaining conflicting file, compute the local patch (`base` to `ours`) and the upstream patch (`base` to `theirs`) and classify:

| Class | Rule (hunk and line counts, per file) | Recommendation |
|---|---|---|
| `disjoint-hunks` | the changed regions do not overlap, but the files conflict only because of adjacent lines (git reports them as touching) | `merge` the hunks with a one-line context shown for confirmation |
| `local-small` | local patch <= 10 changed lines, upstream patch > 10 | `take-upstream`, then re-apply the saved local patch by hand |
| `upstream-small` | upstream patch <= 10 changed lines, local patch > 10 | `keep-mine` for this file, and show the upstream patch to copy over |
| `both-large` | both patches > 10 changed lines | `keep-mine` for this file until reviewed (no recommendation to discard either side) |
| `delete-vs-modify` | one side deleted the file, the other changed it | `keep-mine` if the local side changed it, otherwise `take-upstream` |
| `binary` | binary file differs on both sides | `keep-mine`; Cali cannot show a diff |
| `frontmatter` | the conflict is inside the `SKILL.md` frontmatter | never auto-recommended `take-upstream`: `name` and `description` drive loading, so Cali prints both versions of the keys |

The thresholds (10 lines) are my starting values (**Decided**); they live in one constant and the tests pin their behavior, so they can be tuned without changing the contract. Each recommendation carries a reason line, for example `RECOMMEND take-upstream (local edit is 3 lines, upstream rewrote 48 lines of this file)`.

**3. How the choice is made.**
- **TTY**: `update --apply` walks the conflicting files one at a time, shows the hunks (ours / base / theirs with line numbers) and the recommendation, and offers `take-upstream`, `keep-mine`, `skip skill`. The recommendation is the preselected default. Non-conflicting files are merged as usual.
- **Not a TTY, or `--json`**: nothing is changed. The output lists each conflict with its class, reason, and the exact command that applies the recommendation, e.g. `cali update code-review --apply --resolve SKILL.md=theirs`.
- **Flags** (per skill): `--resolve <file>=ours|theirs` (repeatable) decides single files; `--take-upstream` and `--keep-mine` apply to every conflicting file of the named skill; `--accept-recommended` applies the recommendation for every conflict, and refuses to run if any conflict is `both-large`, `binary`, or `frontmatter`.
- `keep-mine` on a file keeps the installed file; if all of a skill's conflicts are kept, the lock does not advance for that skill and it stays `outdated`. Advancing the lock while keeping local files would hide the upstream change from future merges, so Cali does not do that for files it did not merge. A skill with a mix of choices advances only if every file was merged or taken from upstream or marked `keep-mine` explicitly with `--resolve ...=ours`; in that case the kept local file becomes a recorded local edit against the new base, which `status` shows as `modified`.

**4. No local edit is ever lost.** Before any `theirs` choice overwrites a file, Cali writes to `<data>/backups/<name>/<UTC-timestamp>/`: the whole previous skill folder, and `local.patch`, the unified diff `base` to `ours`. The output prints that path, so a rejected local edit can be re-applied with `git apply` (git is already required). This is what the "zero local edits lost" success metric tests: every `theirs` path produces a backup containing the old bytes.

## 6. Target paths

| Target | Project scope | Global scope | Source of the path |
|---|---|---|---|
| `claude` | `<project>/.claude/skills/<name>/` | `~/.claude/skills/<name>/` (Windows: `%USERPROFILE%\.claude\skills\`) | Project/user skill locations are standard; the Windows form comes from third-party guides, **Verify** against Claude Code docs. |
| `copilot` | `<project>/.github/skills/<name>/` | `~/.copilot/skills/<name>/` | [VS Code docs](https://code.visualstudio.com/docs/agent-customization/agent-skills.md) and [GitHub Copilot CLI docs](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills) |

Findings that shaped this table:

- Paths are home-relative and identical in shape on macOS, Windows and WSL; the OS only changes what `~` is. Cali must not use `/mnt/c` or `\\wsl$` translation.
- **WSL** (**Decided**): the Linux binary in WSL manages the WSL home and WSL project dirs only. It never writes to the Windows profile. Windows and WSL are two independent installs of the same manifest, each with its own lock result (the lock content is identical).
- VS Code and Visual Studio Copilot read `.github/skills`, `.claude/skills`, `.agents/skills` (project) and `~/.copilot/skills`, `~/.claude/skills`, `~/.agents/skills` (personal). The Copilot CLI documents `.github/skills`, `.claude/skills`, `.agents/skills` and `~/.copilot/skills`, `~/.agents/skills`. So `.github/skills` + `~/.copilot/skills` is the one pair every documented Copilot client reads (**Decided** as the `copilot` target).
- **Verify (V1) JetBrains**: I found no documentation of where the JetBrains Copilot plugin reads skills from. The brief asks for JetBrains paths; they cannot be answered from documentation. The walking skeleton checks it by hand on the user's JetBrains IDE. Until then JetBrains is not claimed as supported.
- **Verify (V2) duplicates**: with both targets enabled, VS Code Copilot can see the same skill via `.claude/skills` and `.github/skills`. The docs say how duplicates across workspace roots are resolved, not for this case. If it shows twice, the fallback is a manifest option `copilot_path` / `targets: [claude]` for VS Code-only users; the CLI case still needs `~/.copilot/skills`.
- The target table is a single data structure in code, so adding a path (JetBrains, `.agents/skills`) is a one-line change. Per-target path override in `cali.yaml` is not built in the MVP; add it only if V1/V2 require it.
- `CLAUDE_CONFIG_DIR` is ignored (a reported issue says Claude Code still loads skills from `~/.claude/skills` when it is set).

Each target has its own copy and its own edit state. A local edit in the `claude` copy is not propagated to the `copilot` copy; `status` and `update` treat them independently with the same base.

## 7. Dependencies

The Agent Skills frontmatter has no dependency field, and a top-level unknown key such as `requires:` risks being rejected by stricter loaders (the VS Code docs list only `name`, `description`, `argument-hint`, `user-invocable`, `disable-model-invocation`, `context`). **Decided**: two places, merged as a union.

1. In the skill, `SKILL.md` frontmatter: `metadata:` with a `cali-requires` string value, comma-separated skill names:
   ```yaml
   metadata:
     cali-requires: "writing-plans, test-driven-development"
   ```
   (`metadata` is the spec's extension field. Real skills also keep lists and maps there, e.g. awesome-copilot's `acquire-codebase-knowledge`, so Cali reads only string values and ignores the rest; `cali-requires` itself must be a string. **Verify** that VS Code/Claude Code ignore `cali-requires` without warning.) This travels with the skill, so skills Roj authors in `agent-skills` carry their own dependencies.
2. In `cali.yaml`, `requires: [..]` on the entry, for upstream skills that cannot be edited. It survives updates because it is not inside the skill folder.

Semantics: names only, no versions. A dependency is satisfied when a skill with that name is installed for the same target in the same scope or in global scope. `status` prints `WARN <skill> requires <dep> (missing)`; nothing is installed, and no cycle or transitive logic exists (non-goal 3). Reading `cali-requires` from the installed `SKILL.md` means a local edit to it takes effect immediately.

## 8. Name clashes

- Within a scope and target, an install name is unique (the manifest is a map). `add`/`sync` of a second source with an existing name fails: `name "x" already installed from <source>; remove it first`. To switch a skill's source, run `cali remove x` (it backs the folders up first; `--force` when they have local edits) and then `cali add <new spec>`.
- No alias option: the folder name has to equal the frontmatter `name`, and rewriting `SKILL.md` would make every install look edited.
- Project vs global with the same name: allowed. `status` marks the global one `shadowed` (informational), since the project skill takes precedence in Claude Code.
- A folder that already exists in the target and is not in the manifest (including APM leftovers) is `unmanaged`: `add`/`sync` never overwrite it. `--adopt` takes it over: if it is identical to upstream it just becomes managed; if it differs, it becomes managed and shows as `modified` against the pinned version.

## 9. `import apm.yml` and APM leftovers

- Import reads `apm.yml` and writes the matching `cali.yaml` entries (source, ref). Entries that are not skills (MCP servers, instructions) are skipped with one warning each (non-goal 7).
- Skills APM installed that are not in `apm.yml`: **Decided** Cali never deletes anything it did not install. They appear as `unmanaged` in `status` and `list`, and removing them is the user's job. `--adopt` (section 8) is the way to take one over.
- Not known from the repo: the schema of `apm.yml` and how APM marks its own installs. I have not seen either. The plan must start the import task by reading Roj's real `apm.yml` and writing the mapping from that, and must not guess fields.

## 10. Commands (behavior contracts)

| Command | Behavior |
|---|---|
| `init [-g]` | Create `cali.yaml` with `version: 1` and the detected targets. No network. |
| `add <spec> [--skill n] [--all] [-g] [--target t]` | Fetch source; if no `//path`, scan for `SKILL.md` folders (max depth 4) and ask, or use `--skill/--all`. Writes manifest, lock, pristine, installed copies. |
| `list [-g]` | Installed skills, source, short commit, targets. Offline. |
| `show <name>` | Resolved source, locked commit/tree, paths per target, requires. Offline. |
| `find <text>` | Search `name` + `description` of skills in repos listed under `sources:` and in already installed sources, using the local mirrors (fetched if stale). There is no registry (non-goal 5). |
| `status [--offline] [--exit-code]` | One row per skill x target: `ok`, `modified`, `outdated`, `missing`, `base-lost`, `unlocked` (in the manifest, not yet in the lock), `unmanaged`, `shadowed`, plus dependency warnings. Fetches by default (needed to see new upstream commits); `--offline` skips it. Exit 0 unless `--exit-code` is given. |
| `update [names] [--apply] [--resolve f=ours\|theirs] [--take-upstream] [--keep-mine] [--accept-recommended]` | Section 5: dry run by default; conflicts get a classification and a recommendation. A skill whose ref or folder is gone upstream, or whose upstream folder fails validation, is skipped; the rest is planned or applied, then the exit code is non-zero. A manifest skill that is not locked is noted (run `sync`) and also makes the exit code non-zero. |
| `remove <name>` | Back up the installed folders, remove them, drop the manifest and lock entries. Blocks if the folder has local edits unless `--force`. |
| `sync [--frozen]` | Make installed folders match the lock; resolve manifest entries missing from the lock and write them. `--frozen` fails instead of changing the lock. Never overwrites a `modified` or `unmanaged` folder (reports it). A skill whose upstream folder fails validation, or whose ref or folder is gone upstream before it is locked, is skipped, with its lock entry and folders left as they were; the rest is installed, then the exit code is non-zero. Fetch, git and write errors still stop sync. |
| `import apm.yml` | Section 9. |

Global rules: `outdated` means the tree SHA of `path` at the tracked ref differs from the locked tree. Network failures degrade to a warning in `status`. Output: styled tables and spinners on a TTY, plain text and `--json` when piped; `--json` carries a `schema_version` so the plan can fix its shape in the requirements.

### Git and authentication

- **Decided**: Cali runs the system `git` binary rather than embedding a git library: no dependency, credential helpers work as on the command line, and Git Bash already provides it. Cost: `git` must be installed (Git Bash, WSL Ubuntu and macOS all have it).
- Tokens come from `GITHUB_TOKEN`/`GH_TOKEN`, else `gh auth token`. They are passed to git through `GIT_CONFIG_COUNT` / `GIT_CONFIG_KEY_n` / `GIT_CONFIG_VALUE_n` environment variables, never on the command line (process lists) and never written to the manifest, lock or logs. Only sent to `github.com`.
- Fetches use `--filter=blob:none` first and fall back to a normal fetch if the server refuses it.

## 11. Anti-goals, deferred, and ADRs

Anti-goals: silently losing a local edit; writing a lock whose bytes depend on the OS; Cali touching anything outside the target skill folders, `<data>`, `<cache>`, `<config>`, and the project's `cali.yaml`/`cali.lock`.

Anti-goal (Roj): conflict markers inside installed skill files. Cali recommends and asks instead (section 5).

Deferred (not built in the MVP): `copilot_path` override; JetBrains target; `.agents/skills` target; a file lock for concurrent Cali processes (writes are atomic via temp file + rename, which prevents corruption but not lost updates).

New ADRs required before code, per `CLAUDE.md` rule "No new dependency, datastore or service without a new ADR":
1. Shell out to system `git`, and use `git merge-file` for merges.
2. YAML library for `cali.yaml` (only dependency the formats force).
3. CLI framework.
4. Terminal styling/spinner/prompt library for the `sbx`-style output.
5. On-disk layout: `<config>`, `<data>`, `<cache>`, and the pristine store (sections 2 and 5).

## 12. Left to the builder

Package layout, library choices (via the ADRs above), exact JSON output shapes, the text of messages, hunk-printing format, the file-type check for "text", and the order of tasks. Walking skeleton scope is unchanged from the brief (one skill, one git repo, one agent folder, CI green) and must also settle V1 (JetBrains) and V2 (duplicates) before `copilot` is declared supported.

## 13. How the brief's questions were closed

| Open question | Answer |
|---|---|
| How do skills declare dependencies? | Section 7: `metadata.cali-requires` in `SKILL.md` plus `requires:` in `cali.yaml`, union, names only. |
| Copilot paths for VS Code / JetBrains per OS | Section 6: `.github/skills` and `~/.copilot/skills`, same shape on every OS. VS Code/CLI documented; JetBrains undocumented, V1. |
| Monorepo subfolder, history per subfolder or per repo? | Per repo, with change detection per subfolder: the lock pins one repo commit plus the subfolder's tree SHA; a skill is outdated only if its own tree changed (sections 1, 4, 5). |
| Where do pristine copies and cache live? | Section 2 and 5: pristine in `<data>`, git mirrors in `<cache>`. |
| APM leftovers on import | Section 9: never deleted, reported `unmanaged`. |
| Name clashes | Section 8: error (remove, then add the new source), no aliases; project shadows global. |

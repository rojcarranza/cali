# Graph Report - c6e25ece  (2026-11-01)

## Corpus Check
- 91 files · ~82,749 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 961 nodes · 3264 edges · 28 communities (23 shown, 5 thin omitted)
- Extraction: 82% EXTRACTED · 18% INFERRED · 0% AMBIGUOUS · INFERRED: 602 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `7b959e4d`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- Repo
- Add
- setup
- Milestone 1: Walking skeleton (target 2026-10-30)
- fixture
- io.Writer
- classify_test.go
- testing.T
- Cali Brief
- manifest_test.go
- Start
- Cali design
- Validate
- Using cali
- ADR 0006: Copilot target paths
- ADR 0003: CLI framework
- ADR 0004: Terminal styling, spinner and prompts
- ADR 0007: Same-repo sources
- ParseWith
- dirs_test.go
- golangci-lint v2 config (standard linters)
- Tree
- install.sh
- block-protected.sh
- post-edit-go.sh
- stop-tests.sh
- github.com/rojcarranza/cali
- ADR 0008: Default global manifest

## God Nodes (most connected - your core abstractions)
1. `Add()` - 58 edges
2. `newEnv()` - 53 edges
3. `Sync()` - 49 edges
4. `run()` - 47 edges
5. `setup()` - 45 edges
6. `monorepo()` - 41 edges
7. `Repo` - 39 edges
8. `fileURL()` - 37 edges
9. `Tree` - 37 edges
10. `Read()` - 32 edges

## Surprising Connections (you probably didn't know these)
- `test-auditor agent` --semantically_similar_to--> `Scratch HOME / CALI_*_DIR isolated try-out`  [INFERRED] [semantically similar]
  .claude/agents/test-auditor.md → SETUP.md
- `Scratch HOME / CALI_*_DIR isolated try-out` --conceptually_related_to--> `ADR 0005: On-disk layout (config/data/cache)`  [INFERRED]
  SETUP.md → docs/adr/0005-on-disk-layout.md
- `main()` --calls--> `Execute()`  [EXTRACTED]
  cmd/cali/main.go → internal/cli/cli.go
- `Cali CLI` --references--> `ADR 0005: On-disk layout (config/data/cache)`  [INFERRED]
  README.md → docs/adr/0005-on-disk-layout.md
- `Cali Brief` --rationale_for--> `Cali CLI`  [INFERRED]
  docs/brief.md → README.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Read-only review subagents** — _claude_agents_architecture_guardian_architecture_guardian, _claude_agents_security_reviewer_security_reviewer, _claude_agents_test_auditor_test_auditor [EXTRACTED 1.00]
- **Safe update: pristine base + git merge-file + conflict flags** — docs_adr_0005_on_disk_layout_pristine_store, docs_adr_0001_system_git_git_merge_file, readme_update_conflict_resolution, docs_brief_safe_updates [INFERRED 0.85]

## Communities (28 total, 5 thin omitted)

### Community 0 - "Repo"
Cohesion: 0.06
Nodes (55): Hit, entry, context.Context, os/exec.Cmd, Search(), repo(), repoFilter(), skill() (+47 more)

### Community 1 - "Add"
Cohesion: 0.10
Nodes (90): Action, env, Report, Add(), checkScopeFiles(), LockCommit(), place(), exists() (+82 more)

### Community 2 - "setup"
Cohesion: 0.12
Nodes (59): Locate(), ReadWith(), backupDir(), conflictAndClean(), exact(), failSecondStaging(), Options, fx (+51 more)

### Community 3 - "Milestone 1: Walking skeleton (target 2026-10-30)"
Cohesion: 0.08
Nodes (25): Cali MVP Implementation Plan, Global Constraints, Milestone 1: Walking skeleton (target 2026-10-30), Milestone 2: Daily commands (target 2026-11-13), Milestone 3: Safe update and APM import (target 2026-11-27), Review Focus, Self-review notes, Task 10: `status` (+17 more)

### Community 4 - "fixture"
Cohesion: 0.09
Nodes (55): Check(), Warning, V, Requires(), sortedKeys(), TestCheckMissingReportedPerTarget(), TestCheckSatisfiedByGlobal(), TestRequiresEmptyMetadata() (+47 more)

### Community 5 - "io.Writer"
Cohesion: 0.08
Nodes (46): listRow, state, main(), TestMainExitsZeroOnVersion(), github.com/spf13/cobra.Command, io.Writer, agentFromEnv(), agentPreRun() (+38 more)

### Community 6 - "classify_test.go"
Cohesion: 0.05
Nodes (98): huhPrompter, allDisjoint(), binary(), ChangedLines(), Classify(), edits(), frontmatter(), Class (+90 more)

### Community 7 - "testing.T"
Cohesion: 0.06
Nodes (95): testing.T, TestAgentModeNeverPromptsAndExits2OnUsage(), TestAgentModeTotalsAndHints(), TestShippedSkillHasFrontmatter(), agentSkills(), installedAsDeclared(), TestAgentSkillsFrozenSync(), TestAgentSkillsLockIsCurrent() (+87 more)

### Community 8 - "Cali Brief"
Cohesion: 0.07
Nodes (36): architecture-guardian agent, security-reviewer agent, test-auditor agent, Cali project rules (TDD, ADRs, PR workflow), 3-way merge via git merge-file --diff3, ADR 0001: Shell out to system git, Token via GIT_CONFIG_COUNT env, github.com only, ADR 0002: go.yaml.in/yaml/v3 for cali.yaml (+28 more)

### Community 9 - "manifest_test.go"
Cohesion: 0.09
Nodes (53): appendPair(), copyComments(), ensureMapping(), Manifest, Skill, yaml.Node, markFlow(), Read() (+45 more)

### Community 10 - "Start"
Cohesion: 0.09
Nodes (26): bytes.Buffer, sync.Mutex, sync.Once, sync.WaitGroup, time.Duration, ascii(), at(), cell() (+18 more)

### Community 11 - "Cali design"
Cohesion: 0.10
Nodes (21): 10. Commands (behavior contracts), 11. Anti-goals, deferred, and ADRs, 12. Left to the builder, 13. How the brief's questions were closed, 1. Model, 2. Files and where they live, 3. `cali.yaml` (manifest, hand-editable), 4. `cali.lock` (generated, deterministic) (+13 more)

### Community 12 - "Validate"
Cohesion: 0.24
Nodes (11): os.FileMode, ensureDirs(), Tree, Tree, quote(), safePath(), Validate(), validatePath() (+3 more)

### Community 13 - "Using cali"
Cohesion: 0.50
Nodes (3): Common tasks, Rules, Using cali

### Community 14 - "ADR 0006: Copilot target paths"
Cohesion: 0.33
Nodes (5): ADR 0006: Copilot target paths, Context, Decision, Findings, Questions

### Community 15 - "ADR 0003: CLI framework"
Cohesion: 0.40
Nodes (4): ADR 0003: CLI framework, Consequences, Context, Decision

### Community 16 - "ADR 0004: Terminal styling, spinner and prompts"
Cohesion: 0.40
Nodes (4): ADR 0004: Terminal styling, spinner and prompts, Consequences, Context, Decision

### Community 17 - "ADR 0007: Same-repo sources"
Cohesion: 0.40
Nodes (4): ADR 0007: Same-repo sources, Consequences, Context, Decision

### Community 18 - "ParseWith"
Cohesion: 0.16
Nodes (22): Options, ownerRepo(), Parse(), ParseWith(), reject(), safeScheme(), SameURL(), TestCanonicalURL() (+14 more)

### Community 19 - "dirs_test.go"
Cohesion: 0.22
Nodes (22): FS, Dirs, join(), pick(), Resolve(), resolveWindows(), check(), envOf() (+14 more)

### Community 21 - "Tree"
Cohesion: 0.05
Nodes (73): AddRequest, Installed, plan, Result, scope, Save(), fixedNow(), TestSaveAppendsSuffixOnCollision() (+65 more)

### Community 23 - "install.sh"
Cohesion: 0.83
Nodes (3): die(), say(), install.sh script

### Community 31 - "ADR 0008: Default global manifest"
Cohesion: 0.40
Nodes (4): ADR 0008: Default global manifest, Consequences, Context, Decision

## Knowledge Gaps
- **72 isolated node(s):** `block-protected.sh script`, `post-edit-go.sh script`, `stop-tests.sh script`, `github.com/rojcarranza/cali`, `Context` (+67 more)
  These have ≤1 connection - possible missing edges or undocumented components. (Counts symbols only; 114 node(s) total have ≤1 connection when file, concept and rationale nodes are included.)
- **5 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Repo` connect `Repo` to `Add`, `Tree`?**
  _High betweenness centrality (0.034) - this node is a cross-community bridge._
- **Why does `Add()` connect `Add` to `Repo`, `setup`, `fixture`, `io.Writer`, `manifest_test.go`, `ParseWith`, `Tree`?**
  _High betweenness centrality (0.030) - this node is a cross-community bridge._
- **Why does `Sync()` connect `Add` to `Repo`, `setup`, `fixture`, `io.Writer`, `ParseWith`, `Tree`?**
  _High betweenness centrality (0.028) - this node is a cross-community bridge._
- **Are the 31 inferred relationships involving `Add()` (e.g. with `TestAddAdoptsIdenticalFolder()` and `TestAddDefaultTargetsWritesBoth()`) actually correct?**
  _`Add()` has 31 INFERRED edges - model-reasoned connections that need verification._
- **Are the 30 inferred relationships involving `newEnv()` (e.g. with `TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup()` and `TestRemoveAbortsWhenFolderChangesDuringBackup()`) actually correct?**
  _`newEnv()` has 30 INFERRED edges - model-reasoned connections that need verification._
- **Are the 31 inferred relationships involving `Sync()` (e.g. with `TestGlobalManifestDanglingSymlinkRefused()` and `TestSyncSameRepoBaseLostWhenTreeGone()`) actually correct?**
  _`Sync()` has 31 INFERRED edges - model-reasoned connections that need verification._
- **Are the 28 inferred relationships involving `run()` (e.g. with `TestAgentModeNeverPromptsAndExits2OnUsage()` and `TestAgentModeTotalsAndHints()`) actually correct?**
  _`run()` has 28 INFERRED edges - model-reasoned connections that need verification._
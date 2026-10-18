# Graph Report - same-repo-skill-install-84e54e  (2026-10-18)

## Corpus Check
- 83 files · ~71,431 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 818 nodes · 2830 edges · 26 communities (22 shown, 4 thin omitted)
- Extraction: 82% EXTRACTED · 18% INFERRED · 0% AMBIGUOUS · INFERRED: 510 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `ff9035c5`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- add.go
- Add
- setup
- Milestone 1: Walking skeleton (target 2026-10-30)
- fixture
- io.Writer
- classify_test.go
- testing.T
- Cali Brief
- ParseWith
- Tree
- Cali design
- gitx_test.go
- Write
- ADR 0006: Copilot target paths
- ADR 0003: CLI framework
- ADR 0004: Terminal styling, spinner and prompts
- ADR 0007: Same-repo sources
- ParseFrontmatter
- Resolve
- Validate
- install.sh
- block-protected.sh
- post-edit-go.sh
- stop-tests.sh
- github.com/rojcarranza/cali

## God Nodes (most connected - your core abstractions)
1. `Add()` - 55 edges
2. `newEnv()` - 50 edges
3. `Sync()` - 46 edges
4. `monorepo()` - 41 edges
5. `fileURL()` - 37 edges
6. `Tree` - 36 edges
7. `Repo` - 32 edges
8. `Read()` - 32 edges
9. `setup()` - 32 edges
10. `run()` - 31 edges

## Surprising Connections (you probably didn't know these)
- `test-auditor agent` --semantically_similar_to--> `Scratch HOME / CALI_*_DIR isolated try-out`  [INFERRED] [semantically similar]
  .claude/agents/test-auditor.md → SETUP.md
- `Scratch HOME / CALI_*_DIR isolated try-out` --conceptually_related_to--> `ADR 0005: On-disk layout (config/data/cache)`  [INFERRED]
  SETUP.md → docs/adr/0005-on-disk-layout.md
- `main()` --calls--> `Execute()`  [EXTRACTED]
  cmd/cali/main.go → internal/cli/cli.go
- `CI workflow (vet, lint, test, install script on 3 OSes)` --references--> `golangci-lint v2 config (standard linters)`  [INFERRED]
  .github/workflows/ci.yml → .golangci.yml
- `CI workflow (vet, lint, test, install script on 3 OSes)` --implements--> `Cross-platform constraint (macOS, Windows Git Bash, WSL)`  [INFERRED]
  .github/workflows/ci.yml → docs/brief.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Read-only review subagents** — _claude_agents_architecture_guardian_architecture_guardian, _claude_agents_security_reviewer_security_reviewer, _claude_agents_test_auditor_test_auditor [EXTRACTED 1.00]
- **Safe update: pristine base + git merge-file + conflict flags** — docs_adr_0005_on_disk_layout_pristine_store, docs_adr_0001_system_git_git_merge_file, readme_update_conflict_resolution, docs_brief_safe_updates [INFERRED 0.85]

## Communities (26 total, 4 thin omitted)

### Community 0 - "add.go"
Cohesion: 0.08
Nodes (38): entry, context.Context, os/exec.Cmd, AddRequest, Installed, plan, Result, scope (+30 more)

### Community 1 - "Add"
Cohesion: 0.13
Nodes (77): Action, env, Report, Add(), exists(), fileURL(), git(), isolate() (+69 more)

### Community 2 - "setup"
Cohesion: 0.12
Nodes (59): Locate(), ReadWith(), backupDir(), conflictAndClean(), exact(), failSecondStaging(), Options, fx (+51 more)

### Community 3 - "Milestone 1: Walking skeleton (target 2026-10-30)"
Cohesion: 0.08
Nodes (25): Cali MVP Implementation Plan, Global Constraints, Milestone 1: Walking skeleton (target 2026-10-30), Milestone 2: Daily commands (target 2026-11-13), Milestone 3: Safe update and APM import (target 2026-11-27), Review Focus, Self-review notes, Task 10: `status` (+17 more)

### Community 4 - "fixture"
Cohesion: 0.10
Nodes (48): bytes.Buffer, Check(), Warning, V, Requires(), sortedKeys(), TestCheckMissingReportedPerTarget(), TestCheckSatisfiedByGlobal() (+40 more)

### Community 5 - "io.Writer"
Cohesion: 0.09
Nodes (37): huhPrompter, state, main(), TestMainExitsZeroOnVersion(), github.com/spf13/cobra.Command, io.Writer, deps(), Execute() (+29 more)

### Community 6 - "classify_test.go"
Cohesion: 0.06
Nodes (79): allDisjoint(), binary(), Classify(), edits(), frontmatter(), Class, Recommendation, inFrontmatter() (+71 more)

### Community 7 - "testing.T"
Cohesion: 0.08
Nodes (72): testing.T, agentSkills(), copyFile(), installedAsDeclared(), TestAgentSkillsFrozenSync(), TestAgentSkillsLockIsCurrent(), TestAgentSkillsNewSkill(), gitRun() (+64 more)

### Community 8 - "Cali Brief"
Cohesion: 0.08
Nodes (34): architecture-guardian agent, security-reviewer agent, test-auditor agent, CI workflow (vet, lint, test, install script on 3 OSes), golangci-lint v2 config (standard linters), Cali project rules (TDD, ADRs, PR workflow), 3-way merge via git merge-file --diff3, ADR 0001: Shell out to system git (+26 more)

### Community 9 - "ParseWith"
Cohesion: 0.13
Nodes (37): copyComments(), ensureMapping(), Manifest, Skill, markFlow(), Read(), removeKey(), render() (+29 more)

### Community 10 - "Tree"
Cohesion: 0.05
Nodes (71): Save(), fixedNow(), TestSaveAppendsSuffixOnCollision(), TestSaveCreatesMissingDataDir(), TestSaveDirsArePrivate(), TestSaveRejectsBadName(), TestSaveUserTreeBreakingUpstreamRules(), TestSaveWithoutPatchWritesNoPatchFile() (+63 more)

### Community 11 - "Cali design"
Cohesion: 0.10
Nodes (21): 10. Commands (behavior contracts), 11. Anti-goals, deferred, and ADRs, 12. Left to the builder, 13. How the brief's questions were closed, 1. Model, 2. Files and where they live, 3. `cali.yaml` (manifest, hand-editable), 4. `cali.lock` (generated, deterministic) (+13 more)

### Community 12 - "gitx_test.go"
Cohesion: 0.18
Nodes (29): AuthEnv(), envMap(), TestAuthEnvOnlyGithub(), TestAuthEnvTokenOrder(), TestScrubRemovesCredentials(), exportAt(), fixture(), fixtureSpec() (+21 more)

### Community 13 - "Write"
Cohesion: 0.18
Nodes (14): Entry, Lock, safePath(), entry(), TestMarshalDoesNotEscapeHTML(), TestMarshalNilSkills(), TestMarshalSortedAndStable(), TestReadMissingFile() (+6 more)

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

### Community 18 - "ParseFrontmatter"
Cohesion: 0.19
Nodes (17): Hit, Search(), repo(), repoFilter(), skill(), TestSearchDepthLimit(), TestSearchMatchesDescription(), TestSearchNoRegistry() (+9 more)

### Community 19 - "Resolve"
Cohesion: 0.37
Nodes (12): Dirs, join(), pick(), Resolve(), resolveWindows(), check(), envOf(), TestResolveCaliOverrides() (+4 more)

### Community 20 - "Validate"
Cohesion: 0.24
Nodes (11): os.FileMode, ensureDirs(), Tree, Tree, quote(), safePath(), Validate(), validatePath() (+3 more)

### Community 23 - "install.sh"
Cohesion: 0.83
Nodes (3): die(), say(), install.sh script

## Knowledge Gaps
- **62 isolated node(s):** `block-protected.sh script`, `post-edit-go.sh script`, `stop-tests.sh script`, `github.com/rojcarranza/cali`, `Context` (+57 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **4 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Sync()` connect `Add` to `add.go`, `setup`, `fixture`, `io.Writer`, `ParseWith`, `Tree`, `Write`?**
  _High betweenness centrality (0.032) - this node is a cross-community bridge._
- **Why does `Add()` connect `Add` to `add.go`, `setup`, `fixture`, `io.Writer`, `ParseWith`, `Tree`, `Write`?**
  _High betweenness centrality (0.030) - this node is a cross-community bridge._
- **Why does `Repo` connect `add.go` to `ParseFrontmatter`, `gitx_test.go`?**
  _High betweenness centrality (0.029) - this node is a cross-community bridge._
- **Are the 29 inferred relationships involving `Add()` (e.g. with `TestAddAdoptsIdenticalFolder()` and `TestAddDefaultTargetsWritesBoth()`) actually correct?**
  _`Add()` has 29 INFERRED edges - model-reasoned connections that need verification._
- **Are the 27 inferred relationships involving `newEnv()` (e.g. with `TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup()` and `TestRemoveAbortsWhenFolderChangesDuringBackup()`) actually correct?**
  _`newEnv()` has 27 INFERRED edges - model-reasoned connections that need verification._
- **Are the 28 inferred relationships involving `Sync()` (e.g. with `TestSyncSameRepoLockBehind()` and `TestSyncSameRepoLocksTreeWithoutCommit()`) actually correct?**
  _`Sync()` has 28 INFERRED edges - model-reasoned connections that need verification._
- **Are the 24 inferred relationships involving `monorepo()` (e.g. with `TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup()` and `TestRemoveAbortsWhenFolderChangesDuringBackup()`) actually correct?**
  _`monorepo()` has 24 INFERRED edges - model-reasoned connections that need verification._
# Graph Report - cali  (2026-10-19)

## Corpus Check
- 86 files · ~75,741 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 874 nodes · 3026 edges · 36 communities (32 shown, 4 thin omitted)
- Extraction: 81% EXTRACTED · 19% INFERRED · 0% AMBIGUOUS · INFERRED: 567 edges (avg confidence: 0.85)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `6df1fe4f`
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
- manifest.go
- gitx_test.go
- Cali design
- Equal
- Search
- ADR 0006: Copilot target paths
- ADR 0003: CLI framework
- ADR 0004: Terminal styling, spinner and prompts
- ADR 0007: Same-repo sources
- ParseWith
- dirs_test.go
- plan_test.go
- Tree
- update/update.go
- install.sh
- block-protected.sh
- post-edit-go.sh
- stop-tests.sh
- github.com/rojcarranza/cali
- Validate
- Apply
- plan.go
- ADR 0008: Default global manifest
- Save
- Get
- ADR 0009: Home-relative config directory on every OS
- TestSameLockSameBytes

## God Nodes (most connected - your core abstractions)
1. `Add()` - 58 edges
2. `newEnv()` - 52 edges
3. `Sync()` - 47 edges
4. `run()` - 44 edges
5. `setup()` - 42 edges
6. `monorepo()` - 41 edges
7. `Repo` - 38 edges
8. `fileURL()` - 37 edges
9. `Tree` - 36 edges
10. `Read()` - 32 edges

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

## Communities (36 total, 4 thin omitted)

### Community 0 - "Repo"
Cohesion: 0.07
Nodes (38): entry, context.Context, os/exec.Cmd, AddRequest, Installed, plan, Result, scope (+30 more)

### Community 1 - "Add"
Cohesion: 0.10
Nodes (88): Action, env, Report, Add(), checkScopeFiles(), LockCommit(), place(), exists() (+80 more)

### Community 2 - "setup"
Cohesion: 0.12
Nodes (60): Locate(), backupDir(), conflictAndClean(), exact(), failSecondStaging(), Options, fx, read() (+52 more)

### Community 3 - "Milestone 1: Walking skeleton (target 2026-10-30)"
Cohesion: 0.08
Nodes (25): Cali MVP Implementation Plan, Global Constraints, Milestone 1: Walking skeleton (target 2026-10-30), Milestone 2: Daily commands (target 2026-11-13), Milestone 3: Safe update and APM import (target 2026-11-27), Review Focus, Self-review notes, Task 10: `status` (+17 more)

### Community 4 - "fixture"
Cohesion: 0.11
Nodes (47): bytes.Buffer, Check(), Warning, V, Requires(), sortedKeys(), TestCheckMissingReportedPerTarget(), TestCheckSatisfiedByGlobal() (+39 more)

### Community 5 - "io.Writer"
Cohesion: 0.08
Nodes (46): huhPrompter, listRow, state, main(), TestMainExitsZeroOnVersion(), github.com/spf13/cobra.Command, io.Writer, deps() (+38 more)

### Community 6 - "classify_test.go"
Cohesion: 0.11
Nodes (42): allDisjoint(), binary(), Classify(), edits(), frontmatter(), Class, Recommendation, inFrontmatter() (+34 more)

### Community 7 - "testing.T"
Cohesion: 0.09
Nodes (68): testing.T, agentSkills(), installedAsDeclared(), TestAgentSkillsFrozenSync(), TestAgentSkillsLockIsCurrent(), TestAgentSkillsNewSkill(), gitRun(), run() (+60 more)

### Community 8 - "Cali Brief"
Cohesion: 0.08
Nodes (34): architecture-guardian agent, security-reviewer agent, test-auditor agent, CI workflow (vet, lint, test, install script on 3 OSes), golangci-lint v2 config (standard linters), Cali project rules (TDD, ADRs, PR workflow), 3-way merge via git merge-file --diff3, ADR 0001: Shell out to system git (+26 more)

### Community 9 - "manifest.go"
Cohesion: 0.15
Nodes (33): copyComments(), ensureMapping(), Manifest, Skill, markFlow(), Read(), ReadWith(), removeKey() (+25 more)

### Community 10 - "gitx_test.go"
Cohesion: 0.18
Nodes (29): AuthEnv(), envMap(), TestAuthEnvOnlyGithub(), TestAuthEnvTokenOrder(), TestScrubRemovesCredentials(), Open(), exportAt(), fixture() (+21 more)

### Community 11 - "Cali design"
Cohesion: 0.10
Nodes (21): 10. Commands (behavior contracts), 11. Anti-goals, deferred, and ADRs, 12. Left to the builder, 13. How the brief's questions were closed, 1. Model, 2. Files and where they live, 3. `cali.yaml` (manifest, hand-editable), 4. `cali.lock` (generated, deterministic) (+13 more)

### Community 12 - "Equal"
Cohesion: 0.15
Nodes (27): existing(), Diff(), Equal(), ReadDir(), Tree, skillMD(), static(), symlinkOrSkip() (+19 more)

### Community 13 - "Search"
Cohesion: 0.41
Nodes (10): Hit, Search(), repo(), repoFilter(), skill(), TestSearchDepthLimit(), TestSearchFetchesMissingBlobsInOneFetch(), TestSearchMatchesDescription() (+2 more)

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

### Community 20 - "plan_test.go"
Cohesion: 0.21
Nodes (22): TestClassifyDeleteVsModifyFromPlan(), byPath(), f(), markerLines(), plan(), TestPlanAddAddConflict(), TestPlanBinaryBothChangedConflict(), TestPlanDeleteVsModifyConflict() (+14 more)

### Community 21 - "Tree"
Cohesion: 0.28
Nodes (15): ChangedLines(), TestChangedLines(), TestChangedLinesRefusesBinary(), data(), Plan(), FileEqual(), FileEqualPtr(), File (+7 more)

### Community 22 - "update/update.go"
Cohesion: 0.22
Nodes (12): resolveHint(), short(), FileDecision, jsonConflict, jsonFile, jsonHunk, jsonSkill, jsonTarget (+4 more)

### Community 23 - "install.sh"
Cohesion: 0.83
Nodes (3): die(), say(), install.sh script

### Community 28 - "Validate"
Cohesion: 0.24
Nodes (11): os.FileMode, ensureDirs(), Tree, Tree, quote(), safePath(), Validate(), validatePath() (+3 more)

### Community 29 - "Apply"
Cohesion: 0.22
Nodes (14): MoveAside(), NoSymlinks(), Stage(), EncodeLike(), SameBytes(), TestWriteUncheckedSkipsContentRules(), Apply(), checkFlags() (+6 more)

### Community 30 - "plan.go"
Cohesion: 0.28
Nodes (14): git(), gitEnv(), Action, FilePlan, LocalPatch(), markerSize(), merge3(), mergeFile() (+6 more)

### Community 31 - "ADR 0008: Default global manifest"
Cohesion: 0.40
Nodes (4): ADR 0008: Default global manifest, Consequences, Context, Decision

### Community 32 - "Save"
Cohesion: 0.38
Nodes (9): Save(), fixedNow(), TestSaveAppendsSuffixOnCollision(), TestSaveCreatesMissingDataDir(), TestSaveDirsArePrivate(), TestSaveRejectsBadName(), TestSaveUserTreeBreakingUpstreamRules(), TestSaveWithoutPatchWritesNoPatchFile() (+1 more)

### Community 33 - "Get"
Cohesion: 0.54
Nodes (6): checkTree(), Get(), Put(), TestPutGetRoundTrip(), TestPutRejectsBadTreeSHA(), tree()

### Community 34 - "ADR 0009: Home-relative config directory on every OS"
Cohesion: 0.40
Nodes (4): ADR 0009: Home-relative config directory on every OS, Consequences, Context, Decision

### Community 35 - "TestSameLockSameBytes"
Cohesion: 0.83
Nodes (3): run(), TestSameLockSameBytes(), treeHash()

## Knowledge Gaps
- **68 isolated node(s):** `block-protected.sh script`, `post-edit-go.sh script`, `stop-tests.sh script`, `github.com/rojcarranza/cali`, `Context` (+63 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **4 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Repo` connect `Repo` to `Add`, `gitx_test.go`, `Search`?**
  _High betweenness centrality (0.036) - this node is a cross-community bridge._
- **Why does `Add()` connect `Add` to `Repo`, `Get`, `setup`, `fixture`, `io.Writer`, `manifest.go`, `ParseWith`?**
  _High betweenness centrality (0.031) - this node is a cross-community bridge._
- **Why does `Sync()` connect `Add` to `Repo`, `Get`, `setup`, `TestSameLockSameBytes`, `fixture`, `io.Writer`, `manifest.go`, `Equal`, `ParseWith`?**
  _High betweenness centrality (0.030) - this node is a cross-community bridge._
- **Are the 31 inferred relationships involving `Add()` (e.g. with `TestAddAdoptsIdenticalFolder()` and `TestAddDefaultTargetsWritesBoth()`) actually correct?**
  _`Add()` has 31 INFERRED edges - model-reasoned connections that need verification._
- **Are the 29 inferred relationships involving `newEnv()` (e.g. with `TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup()` and `TestRemoveAbortsWhenFolderChangesDuringBackup()`) actually correct?**
  _`newEnv()` has 29 INFERRED edges - model-reasoned connections that need verification._
- **Are the 29 inferred relationships involving `Sync()` (e.g. with `TestSyncSameRepoBaseLostWhenTreeGone()` and `TestSyncSameRepoLockBehind()`) actually correct?**
  _`Sync()` has 29 INFERRED edges - model-reasoned connections that need verification._
- **Are the 25 inferred relationships involving `run()` (e.g. with `TestAgentSkillsFrozenSync()` and `TestAgentSkillsLockIsCurrent()`) actually correct?**
  _`run()` has 25 INFERRED edges - model-reasoned connections that need verification._
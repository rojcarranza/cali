# Graph Report - cali  (2026-10-18)

## Corpus Check
- 82 files · ~68,663 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 801 nodes · 2800 edges · 28 communities (24 shown, 4 thin omitted)
- Extraction: 83% EXTRACTED · 17% INFERRED · 0% AMBIGUOUS · INFERRED: 480 edges (avg confidence: 0.85)
- Token cost: 189,746 input · 0 output

## Community Hubs (Navigation)
- Git Repo Access
- Install Add Flow
- Lockfile and Update Tests
- MVP Plan and Design Spec
- Dependency Checks and Status
- CLI Commands
- Conflict Classification
- End-to-End CLI Tests
- ADRs Agents and Project Docs
- Manifest YAML Editing
- Skill Tree Diffing
- Merge Plan Tests
- Source Spec Parsing
- Skill Search and Frontmatter
- Tree Index and Line Diffs
- Update Proposal Output
- Safe Path Validation
- Update Apply and Staging
- Three-Way Merge Planner
- Directory Resolution
- Backups
- Pristine Store
- Cross-OS Golden Test
- Install Script
- Protected-File Hook
- Go Post-Edit Hook
- Stop Tests Hook
- Go Module

## God Nodes (most connected - your core abstractions)
1. `Add()` - 52 edges
2. `newEnv()` - 48 edges
3. `monorepo()` - 41 edges
4. `Sync()` - 41 edges
5. `fileURL()` - 37 edges
6. `Tree` - 36 edges
7. `Read()` - 31 edges
8. `setup()` - 31 edges
9. `Repo` - 30 edges
10. `skillMD()` - 30 edges

## Surprising Connections (you probably didn't know these)
- `test-auditor agent` --semantically_similar_to--> `Scratch HOME / CALI_*_DIR isolated try-out`  [INFERRED] [semantically similar]
  .claude/agents/test-auditor.md → SETUP.md
- `Scratch HOME / CALI_*_DIR isolated try-out` --conceptually_related_to--> `ADR 0005: On-disk layout (config/data/cache)`  [INFERRED]
  SETUP.md → docs/adr/0005-on-disk-layout.md
- `main()` --calls--> `Execute()`  [EXTRACTED]
  cmd/cali/main.go → internal/cli/cli.go
- `architecture-guardian agent` --conceptually_related_to--> `Cali project rules (TDD, ADRs, PR workflow)`  [INFERRED]
  .claude/agents/architecture-guardian.md → CLAUDE.md
- `architecture-guardian agent` --references--> `ADR 0001: Shell out to system git`  [EXTRACTED]
  .claude/agents/architecture-guardian.md → docs/adr/0001-system-git.md

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Safe update pipeline (plan, classify, backup, staged swap)** — docs_superpowers_specs_2026_10_16_cali_design_update_algorithm, docs_superpowers_specs_2026_10_16_cali_design_conflict_classification, docs_superpowers_specs_2026_10_16_cali_design_backup_local_patch, docs_superpowers_specs_2026_10_16_cali_design_staged_swap, docs_superpowers_specs_2026_10_16_cali_design_pristine_store [EXTRACTED 1.00]
- **Deterministic reproducible installs** — docs_superpowers_specs_2026_10_16_cali_design_cali_lock, docs_superpowers_specs_2026_10_16_cali_design_tree_sha, docs_superpowers_specs_2026_10_16_cali_design_byte_identical_installs, docs_superpowers_specs_2026_10_16_cali_design_golden_test [INFERRED 0.85]
- **Milestone 3 safe update tasks** — docs_superpowers_plans_2026_10_16_cali_mvp_task_14, docs_superpowers_plans_2026_10_16_cali_mvp_task_15, docs_superpowers_plans_2026_10_16_cali_mvp_task_16, docs_superpowers_plans_2026_10_16_cali_mvp_task_11 [EXTRACTED 1.00]
- **Safe update: pristine base + git merge-file + conflict flags** — docs_adr_0005_on_disk_layout_pristine_store, docs_adr_0001_system_git_git_merge_file, readme_update_conflict_resolution, docs_brief_safe_updates [INFERRED 0.85]
- **Read-only review subagents** — _claude_agents_architecture_guardian_architecture_guardian, _claude_agents_security_reviewer_security_reviewer, _claude_agents_test_auditor_test_auditor [EXTRACTED 1.00]
- **Dependency ADRs (yaml, cobra, charm) under no-new-dep-without-ADR rule** — docs_adr_0002_yaml_library_go_yaml_v3, docs_adr_0003_cli_framework_cobra, docs_adr_0004_terminal_styling_charm_libraries, claude_cali_project_rules [INFERRED 0.85]

## Communities (28 total, 4 thin omitted)

### Community 0 - "Git Repo Access"
Cohesion: 0.06
Nodes (64): entry, context.Context, os/exec.Cmd, AddRequest, Installed, plan, Result, scope (+56 more)

### Community 1 - "Install Add Flow"
Cohesion: 0.12
Nodes (76): Action, env, Report, Add(), checkScopeFiles(), place(), exists(), fileURL() (+68 more)

### Community 2 - "Lockfile and Update Tests"
Cohesion: 0.11
Nodes (70): testing.T, Locate(), entry(), TestMarshalDoesNotEscapeHTML(), TestMarshalNilSkills(), TestMarshalSortedAndStable(), TestReadMissingFile(), TestReadRejectsBadInput() (+62 more)

### Community 3 - "MVP Plan and Design Spec"
Cohesion: 0.06
Nodes (66): Cali MVP implementation plan, Global constraints, Milestone 1: Walking skeleton, Milestone 2: Daily commands, Milestone 3: Safe update and APM import, Review focus, Task 1: ADRs, module, CI, Task 10: status (+58 more)

### Community 4 - "Dependency Checks and Status"
Cohesion: 0.10
Nodes (48): bytes.Buffer, Check(), Warning, V, Requires(), sortedKeys(), TestCheckMissingReportedPerTarget(), TestCheckSatisfiedByGlobal() (+40 more)

### Community 5 - "CLI Commands"
Cohesion: 0.09
Nodes (37): huhPrompter, state, main(), TestMainExitsZeroOnVersion(), github.com/spf13/cobra.Command, io.Writer, deps(), Execute() (+29 more)

### Community 6 - "Conflict Classification"
Cohesion: 0.11
Nodes (42): allDisjoint(), binary(), Classify(), edits(), frontmatter(), Class, Recommendation, inFrontmatter() (+34 more)

### Community 7 - "End-to-End CLI Tests"
Cohesion: 0.15
Nodes (35): agentSkills(), copyFile(), installedAsDeclared(), TestAgentSkillsFrozenSync(), TestAgentSkillsLockIsCurrent(), TestAgentSkillsNewSkill(), gitRun(), run() (+27 more)

### Community 8 - "ADRs Agents and Project Docs"
Cohesion: 0.07
Nodes (37): architecture-guardian agent, security-reviewer agent, test-auditor agent, CI workflow (vet, lint, test, install script on 3 OSes), golangci-lint v2 config (standard linters), Cali project rules (TDD, ADRs, PR workflow), 3-way merge via git merge-file --diff3, ADR 0001: Shell out to system git (+29 more)

### Community 9 - "Manifest YAML Editing"
Cohesion: 0.15
Nodes (32): copyComments(), ensureMapping(), Manifest, Skill, markFlow(), Read(), ReadWith(), removeKey() (+24 more)

### Community 10 - "Skill Tree Diffing"
Cohesion: 0.15
Nodes (27): existing(), Diff(), Equal(), ReadDir(), Tree, skillMD(), static(), symlinkOrSkip() (+19 more)

### Community 11 - "Merge Plan Tests"
Cohesion: 0.21
Nodes (22): TestClassifyDeleteVsModifyFromPlan(), byPath(), f(), markerLines(), plan(), TestPlanAddAddConflict(), TestPlanBinaryBothChangedConflict(), TestPlanDeleteVsModifyConflict() (+14 more)

### Community 12 - "Source Spec Parsing"
Cohesion: 0.18
Nodes (20): Options, ownerRepo(), Parse(), ParseWith(), reject(), safeScheme(), SameURL(), TestCanonicalURL() (+12 more)

### Community 13 - "Skill Search and Frontmatter"
Cohesion: 0.19
Nodes (17): Hit, Search(), repo(), repoFilter(), skill(), TestSearchDepthLimit(), TestSearchMatchesDescription(), TestSearchNoRegistry() (+9 more)

### Community 14 - "Tree Index and Line Diffs"
Cohesion: 0.28
Nodes (15): ChangedLines(), TestChangedLines(), TestChangedLinesRefusesBinary(), data(), Plan(), FileEqual(), FileEqualPtr(), File (+7 more)

### Community 15 - "Update Proposal Output"
Cohesion: 0.22
Nodes (12): resolveHint(), short(), FileDecision, jsonConflict, jsonFile, jsonHunk, jsonSkill, jsonTarget (+4 more)

### Community 16 - "Safe Path Validation"
Cohesion: 0.24
Nodes (11): os.FileMode, ensureDirs(), Tree, Tree, quote(), safePath(), Validate(), validatePath() (+3 more)

### Community 17 - "Update Apply and Staging"
Cohesion: 0.22
Nodes (14): MoveAside(), NoSymlinks(), Stage(), EncodeLike(), SameBytes(), TestWriteUncheckedSkipsContentRules(), Apply(), checkFlags() (+6 more)

### Community 18 - "Three-Way Merge Planner"
Cohesion: 0.28
Nodes (14): git(), gitEnv(), Action, FilePlan, LocalPatch(), markerSize(), merge3(), mergeFile() (+6 more)

### Community 19 - "Directory Resolution"
Cohesion: 0.37
Nodes (12): Dirs, join(), pick(), Resolve(), resolveWindows(), check(), envOf(), TestResolveCaliOverrides() (+4 more)

### Community 20 - "Backups"
Cohesion: 0.38
Nodes (9): Save(), fixedNow(), TestSaveAppendsSuffixOnCollision(), TestSaveCreatesMissingDataDir(), TestSaveDirsArePrivate(), TestSaveRejectsBadName(), TestSaveUserTreeBreakingUpstreamRules(), TestSaveWithoutPatchWritesNoPatchFile() (+1 more)

### Community 21 - "Pristine Store"
Cohesion: 0.54
Nodes (6): checkTree(), Get(), Put(), TestPutGetRoundTrip(), TestPutRejectsBadTreeSHA(), tree()

### Community 22 - "Cross-OS Golden Test"
Cohesion: 0.83
Nodes (3): run(), TestSameLockSameBytes(), treeHash()

### Community 23 - "Install Script"
Cohesion: 0.83
Nodes (3): die(), say(), install.sh script

## Knowledge Gaps
- **20 isolated node(s):** `block-protected.sh script`, `post-edit-go.sh script`, `stop-tests.sh script`, `github.com/rojcarranza/cali`, `Install scope (project/global)` (+15 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **4 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `Sync()` connect `Install Add Flow` to `Git Repo Access`, `Lockfile and Update Tests`, `Dependency Checks and Status`, `CLI Commands`, `Manifest YAML Editing`, `Skill Tree Diffing`, `Source Spec Parsing`, `Pristine Store`, `Cross-OS Golden Test`?**
  _High betweenness centrality (0.030) - this node is a cross-community bridge._
- **Why does `Tree` connect `Tree Index and Line Diffs` to `Git Repo Access`, `Install Add Flow`, `Lockfile and Update Tests`, `Dependency Checks and Status`, `Skill Tree Diffing`, `Merge Plan Tests`, `Update Proposal Output`, `Update Apply and Staging`, `Three-Way Merge Planner`, `Backups`, `Pristine Store`, `Cross-OS Golden Test`?**
  _High betweenness centrality (0.029) - this node is a cross-community bridge._
- **Why does `Add()` connect `Install Add Flow` to `Git Repo Access`, `Lockfile and Update Tests`, `Dependency Checks and Status`, `CLI Commands`, `Manifest YAML Editing`, `Source Spec Parsing`, `Pristine Store`?**
  _High betweenness centrality (0.029) - this node is a cross-community bridge._
- **Are the 28 inferred relationships involving `Add()` (e.g. with `TestAddAdoptsIdenticalFolder()` and `TestAddDefaultTargetsWritesBoth()`) actually correct?**
  _`Add()` has 28 INFERRED edges - model-reasoned connections that need verification._
- **Are the 25 inferred relationships involving `newEnv()` (e.g. with `TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup()` and `TestRemoveAbortsWhenFolderChangesDuringBackup()`) actually correct?**
  _`newEnv()` has 25 INFERRED edges - model-reasoned connections that need verification._
- **Are the 24 inferred relationships involving `monorepo()` (e.g. with `TestRemoveAbortsOnLineEndingOnlyChangeDuringBackup()` and `TestRemoveAbortsWhenFolderChangesDuringBackup()`) actually correct?**
  _`monorepo()` has 24 INFERRED edges - model-reasoned connections that need verification._
- **Are the 23 inferred relationships involving `Sync()` (e.g. with `checkScopeFiles()` and `exportSkill()`) actually correct?**
  _`Sync()` has 23 INFERRED edges - model-reasoned connections that need verification._
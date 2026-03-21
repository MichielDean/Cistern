# Context

## Item: ci-6al33

**Title:** Reduce sandbox disk multiplier: shared object store via git --local clone or proper worktrees
**Status:** in_progress
**Priority:** 2

### Description

Current: N aqueducts per repo = N full independent clones. For a large repo with 5 aqueducts, the disk cost multiplies 5×. At 100 aqueducts this becomes prohibitive.

## Root cause of original worktree failures (commit 60216f8)
The failure was NOT an inherent git worktree limitation. It was two specific bugs:
1. Branch contention: two aqueducts could check out the same branch, causing 'already in use' errors
2. Stale worktree registrations: paths that no longer existed were still registered

Both are fixable. The switch to dedicated clones over-corrected.

## Research results (tested)

### Option A: git worktrees with --detach (RECOMMENDED)
One primary clone per repo. Each aqueduct slot gets a worktree added with --detach:
  git worktree add --detach ~/.cistern/sandboxes/<repo>/<aqueduct>

Working tree is isolated. Branches are checked out per-step (feat/<droplet-id>). No two worktrees ever share a branch. Object store is shared — no duplication.

Disk cost measured:
- Primary clone (ScaledTest): 16 MB (objects + working tree)
- Each additional worktree: 4.7 MB (working tree only — objects shared)
- 100 aqueducts: 16 MB + 100×4.7 MB = ~487 MB (vs current 100×16 MB = 1.6 GB)

Startup fix needed: git worktree prune --expire=0 on every startup to clear stale registrations before adding new ones.

### Option B: git clone --local (object hardlinks)
One reference clone per repo. Additional clones via git clone --local (hardlinks, not copies):
  git clone --local ~/.cistern/sandboxes/<repo>/_ref ~/.cistern/sandboxes/<repo>/<aqueduct>

Disk cost: identical to Option A (4.7 MB per aqueduct). Advantage: completely independent .git dirs — no worktree registration issues. Disadvantage: aqueduct clones must fetch from remote (not reference) because hardlinks are point-in-time, not live.

## Recommended implementation: Option A (worktrees with --detach)

Changes to internal/cataractae/sandbox.go:
1. EnsureDedicatedClone → EnsurePrimaryClone: ensure ~/.cistern/sandboxes/<repo>/_primary/ exists (full clone)
2. EnsureWorktree: ACTUALLY implement this — git worktree add --detach <path> if not already registered; git worktree prune --expire=0 first to clear stale entries
3. PrepareBranch: unchanged — checkout feat/<droplet-id> in the worktree as before
4. On startup in runner.go: prune stale worktrees before registering new ones

Changes to runner.go:
- Replace EnsureDedicatedClone(w.SandboxDir) with EnsurePrimaryClone + EnsureWorktree
- Primary clone dir: filepath.Join(sandboxBase, '_primary')
- Worktree dirs: filepath.Join(sandboxBase, workerName) — same paths as current

Migration: existing dedicated clones at ~/.cistern/sandboxes/<repo>/<aqueduct>/ can stay as-is. On next startup, runner detects no primary clone and converts: treats first aqueduct's existing clone as primary, worktree-adds the rest. Or simpler: just delete sandboxes and let them re-clone. Doctor can detect and suggest migration.

This is backward compatible — worktree directories have the same paths as current dedicated clone directories.

## Current Step: simplify

- **Type:** agent
- **Role:** simplifier
- **Context:** full_codebase

## Recent Step Notes

### From: manual

Added tests for the three previously uncovered functions:

1. EnsureWorktree (internal/cataractae/sandbox_test.go): 4 tests covering creation, idempotency, legacy clone replacement, and stale-registration pruning. Each test uses a real git repo.

2. prepareBranchInSandbox (internal/castellarius/branch_lifecycle_test.go): 3 tests covering new branch creation from origin/main, git identity configuration, and branch resumption (verifies existing commits are preserved).

3. cleanupBranchInSandbox (internal/castellarius/branch_lifecycle_test.go): 2 tests covering branch deletion + HEAD detach, and best-effort no-op when branch is missing.

All 9 test packages pass.

### From: manual

Added tests for all three previously uncovered functions. All 9 packages pass.

### From: scheduler

Implement pass rejected: HEAD has not advanced since last review (commit: 6eb2462665f0c1219f6f5948b9575f18dad6d47e). No new commits were found. You must commit your changes before signaling pass.

### From: manual

Committed the 9 test files (sandbox_test.go, branch_lifecycle_test.go) that were written but untracked. All 9 packages pass. HEAD now at 90a5ff8.

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>.claude/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>code-simplifier</name>
    <description>code-simplifier</description>
    <location>.claude/skills/code-simplifier/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-6al33

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-6al33
    ct droplet recirculate ci-6al33 --to implement

**Block (genuinely blocked, cannot proceed):**
    ct droplet block ci-6al33

Add notes before signaling:
    ct droplet note ci-6al33 "What you did / found"

The `ct` binary is on your PATH.

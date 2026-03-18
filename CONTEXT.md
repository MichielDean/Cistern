# Context

## Item: ci-a2pzc

**Title:** Phantom commit prevention: verify implement always advances HEAD before routing to review
**Status:** in_progress
**Priority:** 2

### Description

The implementer can signal pass without having committed any changes. The scheduler routes to the reviewer purely on outcome=pass, never checking the sandbox state. The reviewer then sees the same diff it already reviewed and correctly recirculates — but the cycle repeats indefinitely.

## Root cause (confirmed by code review)

1. runner.go spawns the implementer tmux session — returns immediately
2. Implementer edits files, may fail to commit (git commit exits non-zero, wrong CWD, nothing staged) — no detection
3. Implementer calls `ct droplet pass` — scheduler sees outcome=pass, routes to review
4. generateDiff() in context.go runs `git diff origin/main...HEAD` — produces the same diff as before (HEAD did not advance)
5. Reviewer correctly finds issue still present, recirculates
6. Loop forever

## Fix: record reviewed commit hash; verify advancement before routing to review

### Schema change
Add column to droplets table (additive migration, safe on existing DBs):
```sql
ALTER TABLE droplets ADD COLUMN last_reviewed_commit TEXT;
```

### runner.go: store HEAD when generating diff
In prepareDiffOnly(), after generateDiff() succeeds, capture and store HEAD:
```go
head, _ := currentHead(sandboxDir)  // git rev-parse HEAD
queue.SetLastReviewedCommit(itemID, head)
```

### scheduler.go: verify HEAD advanced before routing implement → review
In observeRepo(), when routing from implement (outcome=pass):
- Read item.LastReviewedCommit from DB
- If LastReviewedCommit is non-empty (revision cycle, not first pass):
  - Run git rev-parse HEAD in sandbox
  - If HEAD == LastReviewedCommit: auto-recirculate to implement with note:
    "Implement pass rejected: HEAD has not advanced since last review (commit: <hash>). No new commits were found. You must commit your changes before signaling pass."
  - If HEAD != LastReviewedCommit: allow routing to review (as normal)
- If LastReviewedCommit is empty: first implement pass, route normally

### cistern/client.go
Add SetLastReviewedCommit(dropletID, commitHash string) error
Add GetLastReviewedCommit(dropletID string) (string, error)
Expose LastReviewedCommit string field on Droplet struct

### sandbox.go
Add func currentHead(dir string) (string, error) — runs git rev-parse HEAD, returns hash

### Implementer pre-pass checklist (feature.yaml)
Add as step (a.1) — runs BEFORE git add/commit:
```bash
BEFORE=$(git rev-parse HEAD)
git add -A
git status --short  # show exactly what is staged
git commit -m "<droplet-id>: <description>"
AFTER=$(git rev-parse HEAD)
if [ "$BEFORE" = "$AFTER" ]; then
  echo "ERROR: commit failed — HEAD did not advance. Check git status."
  # signal recirculate, do NOT pass
  ct droplet recirculate <id> --notes "Commit failed: HEAD did not advance after git commit. Nothing was staged."
  exit 1
fi
```

## What this guarantees
- A reviewer will never see the same diff twice
- If the implementer fails to commit (any reason), the system auto-recirculates with a clear message
- The LLM instruction is a belt; the runner check is the suspenders

## Constraints
- Migration must be additive (ALTER TABLE ADD COLUMN — safe on existing DBs)
- currentHead() is a pure helper, no side effects
- The runner check only applies when routing implement→review, not other transitions
- Auto-recirculate note must include the commit hash so the implementer can diagnose
- All existing tests must pass; new tests required for:
  - SetLastReviewedCommit / GetLastReviewedCommit
  - currentHead()
  - scheduler routing: HEAD-not-advanced triggers auto-recirculate
  - scheduler routing: HEAD-advanced routes normally
- go mod tidy must be clean
- Both aqueduct/feature.yaml AND cmd/ct/assets/aqueduct/feature.yaml must be updated

## Current Step: implement

- **Type:** agent
- **Role:** implementer
- **Context:** full_codebase

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>.claude/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>github-workflow</name>
    <description>---</description>
    <location>.claude/skills/github-workflow/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-a2pzc

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-a2pzc
    ct droplet recirculate ci-a2pzc --to implement

**Block (genuinely blocked, cannot proceed):**
    ct droplet block ci-a2pzc

Add notes before signaling:
    ct droplet note ci-a2pzc "What you did / found"

The `ct` binary is on your PATH.

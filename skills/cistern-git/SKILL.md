---
name: cistern-git
description: Git conventions for Cistern aqueduct cataractae. Use for all git operations in sandboxes: staging, committing, diffing, branching, and pushing. Covers rules specific to per-droplet worktrees, CONTEXT.md exclusion, two-dot diff, and no-stash policy.
---

# Cistern Git Conventions

## Worktree model

Each droplet has an isolated worktree at `~/.cistern/sandboxes/<repo>/<droplet-id>/`.
The Castellarius creates and cleans up worktrees. Agents never run `git worktree add/remove`.

## Staging — always exclude CONTEXT.md

CONTEXT.md is written by the Castellarius on every dispatch. Never commit it.

```bash
git add -A -- ':!CONTEXT.md'
git status --short   # verify no CONTEXT.md, no binaries
```

## Committing — verify HEAD advances

```bash
BEFORE=$(git rev-parse HEAD)
git add -A -- ':!CONTEXT.md'
git commit -m "<droplet-id>: <description>"
AFTER=$(git rev-parse HEAD)
if [ "$BEFORE" = "$AFTER" ]; then
  echo "ERROR: nothing staged. Commit failed."
  ct droplet recirculate <id> --notes "Commit failed: HEAD did not advance."
  exit 1
fi
```

## Diffing — always two dots, not three

```bash
# Correct — shows actual changes on this branch
git diff origin/main..HEAD
git diff origin/main..HEAD --name-only

# WRONG — three dots computes merge base diff, appears empty on rebased branches
# git diff origin/main...HEAD   ← never use this
```

To list commits on the branch:
```bash
git log origin/main..HEAD --oneline
```

## No stash

Per-droplet worktrees start clean. **Never run `git stash`** — it silently hides uncommitted work and has caused phantom empty deliveries. If the worktree is dirty before your work, the Castellarius will detect it and recirculate.

## Pushing

```bash
git push origin <branch>
# If rebased:
git push --force-with-lease origin <branch>
```

## Branch name

Your branch is `feat/<droplet-id>`. It is created by the Castellarius. Check with:
```bash
git branch --show-current
```

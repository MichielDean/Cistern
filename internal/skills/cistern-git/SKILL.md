---
name: cistern-git
description: Git conventions for Cistern aqueduct cataractae. Use for all git operations in sandboxes: staging, committing, diffing, branching, and pushing. Covers rules specific to per-droplet worktrees, CONTEXT.md exclusion, merge-base diff, and no-stash policy.
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

## Diffing — always use merge-base

```bash
# Correct — shows only this branch's own changes, regardless of rebase state
git diff $(git merge-base HEAD origin/main)..HEAD
git diff $(git merge-base HEAD origin/main)..HEAD --name-only
```

Two-dot (`git diff origin/main..HEAD`) is wrong for unrebased branches: it includes all commits since the branch diverged, meaning other PRs that merged to main after branching appear in the diff. Merge-base is always correct.

To list commits on the branch:
```bash
git log $(git merge-base HEAD origin/main)..HEAD --oneline
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

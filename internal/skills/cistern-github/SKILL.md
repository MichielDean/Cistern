---
name: cistern-github
description: GitHub CLI operations for Cistern delivery cataractae. Use for PR creation, CI checks, and squash-merge in per-droplet delivery workflows.
---

# Cistern GitHub Operations

## Tools

Use `gh` CLI for all GitHub operations. Prefer CLI over GitHub MCP servers for lower context usage.

## PR Lifecycle

```bash
# Create a PR for the current droplet branch
gh pr create \
  --title "$PR_TITLE" \
  --body "Closes droplet $DROPLET_ID." \
  --base main --head $BRANCH

# If PR already exists
gh pr view $BRANCH --json url --jq '.url'

# Check CI status
gh pr checks $PR_URL

# Squash-merge when all checks pass
gh pr merge $PR_URL --squash --delete-branch

# Confirm merge
gh pr view $PR_URL --json state --jq '.state'   # must be "MERGED"
```

## Conflict Resolution

**Conflicts MUST be resolved automatically. Never stop and ask the user.**

Cistern agents resolve conflicts by keeping both sets of changes. The canonical
protocol is in `cataractae/delivery/INSTRUCTIONS.md` — follow it exactly.

Summary:
1. `git diff --name-only --diff-filter=U` — identify conflicted files
2. For each file: keep what HEAD added AND keep what this branch adds
3. `go build ./...` — verify the merge compiles
4. `git add $(git diff --name-only --diff-filter=U)` — stage resolved files
5. `git rebase --continue`
6. `go build ./... && go test ./...` — verify after full rebase
7. `git push --force-with-lease origin $BRANCH`

Most conflicts are additive: HEAD added X, this branch adds Y — keep both.
Never discard branch additions.

## Cistern Delivery Model

Cistern uses **per-droplet branches** (`feat/<droplet-id>`), not stacked PRs.
Each droplet is independent. There is no stacked-PR workflow.

# Contributing to Cistern

## Branch Protection

The `main` branch has GitHub branch protection configured with the following rules:

- **Require branches to be up to date before merging** (`strict: true` on required status checks) — a PR whose branch is behind `main` cannot be merged until it is rebased onto the current tip of `main`.
- **Required status check:** `build` must pass before merging.

### Why strict up-to-date enforcement is intentional

The delivery cataractae already enforces a mandatory rebase step (Step 2 in `cataractae/delivery/INSTRUCTIONS.md`) before opening or merging a PR. The GitHub branch protection rule is a second, independent layer of defense: even if an agent skips or fails the rebase step, GitHub will block the merge at the platform level with a status check error.

This is defense-in-depth — neither layer alone is sufficient:

| Layer | What it does |
|---|---|
| Delivery INSTRUCTIONS.md Step 2 | Agent rebases branch before creating or merging the PR |
| GitHub branch protection (`strict`) | Platform blocks merge if branch is behind `main` |

Both layers must remain in place. Do not disable the branch protection rule without a clear reason and a replacement control.

### What this means for contributors

Before a PR can be merged:

1. Rebase your branch onto the current tip of `main`:
   ```bash
   git fetch origin main
   git rebase origin/main
   git push --force-with-lease origin <branch>
   ```
2. Ensure the `build` status check passes.

If GitHub shows "This branch is out of date with the base branch", run the rebase above and push again.

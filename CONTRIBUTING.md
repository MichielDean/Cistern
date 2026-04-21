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

## Web UI Development

The Cistern web UI is a React SPA served at `/app/` alongside the xterm.js TUI dashboard at `/`.

### Prerequisites

- Node.js 18+ and npm
- Go 1.22+ (for building the server with embedded assets)

### Build commands

```bash
# Build the React SPA (outputs to cmd/ct/assets/web/)
make web-build

# Run the Vite dev server with API proxy to localhost:5737
make web-dev

# Build the Go binary (includes embedded web assets)
make build
```

### Development workflow

1. Start the Go dashboard server: `ct dashboard --web --addr 0.0.0.0:5737`
2. In a separate terminal, run `make web-dev` for hot-reloaded frontend development
3. The Vite dev server proxies `/api` and `/ws` requests to the Go server

### Architecture

- **Go server** (`cmd/ct/dashboard_web_spa.go`): Embeds the built SPA via `//go:embed assets/web` and serves it under `/app/`. Client-side routing handles all sub-routes.
- **React app** (`web/src/`): React Router routes under `/app/`, SSE for live updates, WebSocket for peek. Dark theme with Tailwind CSS.
- **Tests**: Frontend tests use Vitest + React Testing Library (`npm test`). Go integration tests cover SPA routing and API endpoints.

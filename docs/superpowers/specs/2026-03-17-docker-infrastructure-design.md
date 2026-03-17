# Docker Infrastructure Design

**Date:** 2026-03-17
**Project:** Cistern
**Status:** Approved

## Goals

1. Run Cistern reliably inside a Docker container for portability and reproducible setup.
2. Support full Castellarius operation (tmux + claude sessions) inside the container.
3. Publish multi-arch images to GitHub Container Registry (GHCR) on release.

---

## Files to Create

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build: Go builder + Debian-slim runtime |
| `docker-entrypoint.sh` | Startup script: install/update claude, auth, init, start |
| `.env.example` | Template for secrets (users copy to `.env`, gitignored) |
| `.dockerignore` | Exclude non-build files from Docker build context |
| `.github/workflows/release.yml` | Build + test + publish to GHCR on tag push |

No existing files are modified.

---

## Dockerfile

### Stage 1 — `builder` (`golang:1.26`)

- Copies full source tree
- Ensures `gcc` and `libc6-dev` are available (they are present in the official `golang` image by default)
- Sets `CGO_ENABLED=1` explicitly — required by `go-sqlite3` which uses CGO
- Runs `go mod download`
- Builds both binaries:
  - `go build -o /out/ct ./cmd/ct`
  - `go build -o /out/aqueduct ./cmd/aqueduct`
- The output binaries are **dynamically linked against glibc** (not static). `debian:bookworm-slim` is the correct runtime pairing because both images use glibc from the same Debian family.

### Stage 2 — `runtime` (`debian:bookworm-slim`)

Installed via `apt`:
- `tmux` — session manager for spawning Claude agent sessions
- `git` — repo cloning and worktree operations
- `gh` — GitHub CLI (installed from GitHub's official APT repo with arch-aware keyring: `$(dpkg --print-architecture)` selects `amd64` or `arm64` automatically)
- `nodejs`, `npm` — runtime for Claude CLI install/update
- `openssh-client`, `curl`, `ca-certificates` — supporting tools

Baked-in configuration (as `ENV` directives):
- `GH_CONFIG_DIR=/root/.cistern/auth/gh` — redirects gh auth state into the volume; `GH_CONFIG_DIR` is an officially documented `gh` env var
- `CLAUDE_CONFIG_DIR=/root/.cistern/auth/claude` — **must be verified during implementation** against the installed `@anthropic-ai/claude-code` version. If `CLAUDE_CONFIG_DIR` is not honoured by the claude CLI, the fallback strategy is to bind-mount or symlink `~/.claude` into the volume, or document that users must re-authenticate when creating a new container.
- `PATH` already includes `/usr/local/bin` on Debian, which is where `npm install -g` installs binaries when run as root. This ensures `exec.LookPath("claude")` in the Go code finds the binary correctly without relying on the `$HOME/.local/bin/claude` fallback.

Copied from builder:
- `/usr/local/bin/ct`
- `/usr/local/bin/aqueduct`
- `docker-entrypoint.sh` → `/usr/local/bin/docker-entrypoint.sh` (executable)

Default `CMD`: `ct castellarius start`

---

## docker-entrypoint.sh

Runs on every container start in this order:

1. **Update Claude CLI** — `npm install -g @anthropic-ai/claude-code`
   Always runs; npm is idempotent and updates to the latest version.

2. **GitHub auth check** — `gh auth status 2>/dev/null`
   If not authenticated, runs `gh auth login --web` (device/browser flow, requires `-it`).
   Tokens persist in `/root/.cistern/auth/gh` (inside the volume).
   On headless Linux servers where browser flow is not available, set `GH_TOKEN` in the `.env` file instead — `gh` will use it automatically and skip the interactive prompt.

3. **Claude auth check** — checks for the existence of the claude auth config file at `$CLAUDE_CONFIG_DIR` (or falls back to checking if `ANTHROPIC_API_KEY` is set).
   Note: the exact `claude auth status` command will be verified against the installed claude CLI version during implementation. If an auth-status subcommand is unavailable, a config file existence check is used as the detection mechanism.
   If not authenticated, runs `claude auth login` (OAuth browser flow, requires `-it`).
   State persists in `/root/.cistern/auth/claude` (inside the volume).
   On headless deployments, set `ANTHROPIC_API_KEY` in the `.env` file — claude will use it and skip the auth prompt.

4. **Cistern init** — runs `ct init` unconditionally on every start.
   `ct init` uses `writeFileIfAbsent` internally, which skips files that already exist (printing a warning to stderr for each skipped file — acceptable noise). Running unconditionally avoids partial-init edge cases where `cistern.yaml` exists but `aqueduct/` or `cataractae/` directories are absent.

5. **`exec "$@"`** — hands off to the container command, replacing the shell process.
   SIGTERM/SIGINT propagate directly to `ct`.

**First-run requirement:** On first boot, the container must be started with `-it` to allow interactive browser auth flows. Subsequent starts detect persisted tokens and skip auth automatically.

**Headless / server deployments:** Set `ANTHROPIC_API_KEY` and `GH_TOKEN` in `.env`. The entrypoint skips interactive auth when these are present.

**Command override:** `exec "$@"` means users can run any `ct` subcommand as the container command (e.g., `docker run -it ... ct droplet list`) and still get the install/auth/init steps.

---

## Persistent State

All state lives under `/root/.cistern/` and is covered by a single volume mount:

| Path | Contents |
|------|---------|
| `/root/.cistern/cistern.db` | SQLite droplet database |
| `/root/.cistern/cistern.yaml` | Main configuration |
| `/root/.cistern/aqueduct/` | Workflow YAML files |
| `/root/.cistern/cataractae/` | Role/identity CLAUDE.md files |
| `/root/.cistern/sandboxes/` | Git clones and worktrees |
| `/root/.cistern/auth/gh/` | `gh` auth tokens (via `GH_CONFIG_DIR`) |
| `/root/.cistern/auth/claude/` | Claude auth tokens (via `CLAUDE_CONFIG_DIR`) |

**Named volume (default):**
```bash
docker run -it -v cistern-data:/root/.cistern ghcr.io/michiieldean/cistern
```

**Host bind mount (alternative):**
```bash
docker run -it -v ~/my-cistern:/root/.cistern ghcr.io/michiieldean/cistern
```

---

## Secrets

No secrets are baked into the image. Runtime secrets are passed as environment variables.

`.env.example` documents all supported variables:

```bash
# Required for non-interactive / headless deployments.
# If omitted, the entrypoint will prompt for OAuth login on first run (-it required).
ANTHROPIC_API_KEY=your_anthropic_key_here
GH_TOKEN=your_github_token_here

# Optional overrides — defaults shown.
# CT_DB=/root/.cistern/cistern.db
# CT_CONFIG=/root/.cistern/cistern.yaml
# CLAUDE_PATH=/usr/local/bin/claude
```

Usage:
```bash
cp .env.example .env
# edit .env with your values
docker run -it --env-file .env -v cistern-data:/root/.cistern ghcr.io/michiieldean/cistern
```

If both OAuth tokens (persisted in the volume) and env vars are present, the CLI tools use the env vars (standard behaviour for both `gh` and `claude`).

---

## Release Workflow (`.github/workflows/release.yml`)

**Trigger:** Push of a tag matching `v*` (e.g., `v1.0.0`)

### Job 1 — `test` (ubuntu-latest)

`ubuntu-latest` has `gcc` available by default, satisfying the CGO requirement for `go-sqlite3`.

1. Checkout with `fetch-depth: 0`
2. `actions/setup-go` using `go-version-file: go.mod`
3. Go module cache (`actions/cache`) — same cache key pattern as `pr-build-selfhosted.yml`
4. `go mod download`
5. `go vet ./...`
6. `go build ./...`
7. `go test ./...`

### Job 2 — `publish` (ubuntu-latest, `needs: test`)

1. Checkout
2. `docker/setup-qemu-action` — enables ARM emulation for multi-arch builds
3. `docker/setup-buildx-action`
4. `docker/login-action` — logs into `ghcr.io` using `GITHUB_TOKEN` (automatic, no secrets to configure)
5. `docker/metadata-action` — derives tags from the git tag:
   - `ghcr.io/michiieldean/cistern:1.2.3`
   - `ghcr.io/michiieldean/cistern:1.2`
   - `ghcr.io/michiieldean/cistern:latest`
   Note: GHCR normalises image names to lowercase. The image path `ghcr.io/michiieldean/cistern` intentionally differs in case from the Go module path `github.com/MichielDean/cistern`.
6. `docker/build-push-action` — builds and pushes `linux/amd64` + `linux/arm64`

The existing `pr-build-selfhosted.yml` is unchanged.

---

## Cross-Platform Notes

The container is always a Linux image (`debian:bookworm-slim`). Platform support:

| Host OS | How to run |
|---------|-----------|
| Linux | Docker Engine natively |
| macOS (Intel) | Docker Desktop (`linux/amd64` image) |
| macOS (Apple Silicon) | Docker Desktop (`linux/arm64` image) |
| Windows | Docker Desktop with WSL2 backend |

tmux runs natively inside the Linux container regardless of host OS.

#!/usr/bin/env bash
set -euo pipefail

# ── 1. Install / update opencode CLI ────────────────────────────────────────
echo "==> Updating opencode CLI..."
go install github.com/opencode-ai/opencode@latest

# ── 2. GitHub auth ────────────────────────────────────────────────────────────
if ! gh auth status >/dev/null 2>&1; then
    if [ -n "${GH_TOKEN:-}" ]; then
        echo "==> Authenticating gh with GH_TOKEN..."
        echo "$GH_TOKEN" | gh auth login --with-token
    else
        echo "==> GitHub authentication required (opens browser)."
        echo "    On headless servers set GH_TOKEN in your .env file instead."
        gh auth login --web
    fi
fi

# ── 3. Cistern init (idempotent) ──────────────────────────────────────────────
# ct init uses writeFileIfAbsent internally — existing files are skipped.
# Output is suppressed to avoid noise on every container start.
ct init > /dev/null 2>&1

# ── 4. Hand off ───────────────────────────────────────────────────────────────
exec "$@"
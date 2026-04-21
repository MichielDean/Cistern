# ─── installer-test image ─────────────────────────────────────────────────────
#
# Two-stage image for installer tests.
#
# Stage 1 (builder): compiles ct and fakeagent inside golang:1.26 so the
#   resulting binaries are linked against the same glibc version that ships
#   in debian:bookworm.
#
# Stage 2 (runtime): cistern-systemd-test (systemd as PID 1) + extra tools
#   (git, tmux) + the compiled binaries.  fakeagent is symlinked as
#   /usr/local/bin/opencode so that installer tests can exercise ct doctor /
#   ct init without a real opencode installation or API key.
#
# Build (from repo root — build base image first):
#   docker build -t cistern-systemd-test test/docker/systemd
#   docker build -t cistern-installer-test \
#     -f test/docker/installer-test/Dockerfile .
#
# Run:
#   docker run --privileged --rm -d --name installer-test cistern-installer-test
#   docker exec installer-test systemctl is-system-running

# ─── Stage 1: build ct and fakeagent ─────────────────────────────────────────
FROM golang:1.26 AS builder

WORKDIR /src

# Cache module downloads separately from the source copy so that a source
# change does not invalidate the module layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# ct requires CGO (go-sqlite3 bundles the SQLite amalgamation).
RUN CGO_ENABLED=1 go build -o /out/ct ./cmd/ct

# fakeagent has no C dependencies.
RUN CGO_ENABLED=0 go build -o /out/fakeagent ./internal/testutil/fakeagent

# ─── Stage 2: systemd runtime with test binaries ──────────────────────────────
FROM cistern-systemd-test

# Install extra tools that ct doctor and install.sh check for (git, tmux).
# systemd is already present in the base image.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        git \
        tmux \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install binaries compiled in the builder stage.
COPY --from=builder /out/ct          /usr/local/bin/ct
COPY --from=builder /out/fakeagent   /usr/local/bin/fakeagent

# Stub the opencode CLI with fakeagent.  This satisfies ct doctor's "opencode CLI
# found" check and allows fakeagent-based tests to exercise the agent spawn
# pipeline without a real LLM or API key.
RUN ln -s /usr/local/bin/fakeagent /usr/local/bin/opencode

# Stub the gh CLI so that ct doctor's "gh CLI installed" and "gh authenticated"
# checks succeed without a real GitHub CLI installation or credentials.
RUN printf '#!/bin/sh\nexit 0\n' > /usr/local/bin/gh && chmod +x /usr/local/bin/gh

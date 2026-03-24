# ─── installer-test image ─────────────────────────────────────────────────────
#
# Two-stage image for installer tests.
#
# Stage 1 (builder): compiles ct and fakeagent inside golang:1.26 so the
#   resulting binaries are linked against the same glibc version that ships
#   in debian:bookworm.
#
# Stage 2 (runtime): debian:bookworm + systemd as PID 1 + the compiled
#   binaries.  fakeagent is symlinked as /usr/local/bin/claude so that
#   installer tests can exercise ct doctor / ct init without a real Claude
#   Code installation or API key.
#
# Build (from repo root):
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
FROM debian:bookworm

# ENV container=docker signals systemd to enable container-specific overrides
# and skip hardware-only targets that hang in a containerised environment.
ENV container=docker

# Install systemd and the extra tools that ct doctor and install.sh check for
# (git, tmux).  No Go toolchain is needed at runtime.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        systemd \
        systemd-sysv \
        git \
        tmux \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Mask units that require direct hardware access or kernel interfaces that are
# unavailable inside Docker.  Same set as test/docker/systemd/Dockerfile.
RUN for unit in \
        systemd-udevd.service \
        systemd-udev-settle.service \
        systemd-udev-trigger.service \
        udev.service \
        getty@tty1.service \
        serial-getty@ttyS0.service \
        sys-kernel-debug.mount \
        sys-kernel-tracing.mount \
        systemd-remount-fs.service \
        systemd-machine-id-commit.service \
        systemd-firstboot.service \
        systemd-update-utmp.service \
        systemd-update-utmp-runlevel.service \
    ; do \
        ln -sf /dev/null /etc/systemd/system/${unit}; \
    done

# Send SIGRTMIN+3 on docker stop so systemd shuts down cleanly instead of
# receiving the default SIGTERM which it ignores.
STOPSIGNAL SIGRTMIN+3

# Install binaries compiled in the builder stage.
COPY --from=builder /out/ct          /usr/local/bin/ct
COPY --from=builder /out/fakeagent   /usr/local/bin/fakeagent

# Stub the claude CLI with fakeagent.  This satisfies ct doctor's "claude CLI
# found" check and allows fakeagent-based tests to exercise the agent spawn
# pipeline without a real LLM or API key.
RUN ln -s /usr/local/bin/fakeagent /usr/local/bin/claude

CMD ["/lib/systemd/systemd"]

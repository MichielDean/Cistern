# ─── systemd-capable base image for installer tests ──────────────────────────
#
# WHY --privileged IS REQUIRED
# ─────────────────────────────────────────────────────────────────────────────
# systemd acts as PID 1 and manages the Linux cgroup hierarchy.  Inside a
# normal (non-privileged) Docker container the kernel presents the cgroup
# filesystem as read-only or restricted; systemd detects this and either
# refuses to start or enters a permanently degraded state that cannot
# supervise child services.
#
# --privileged provides:
#   • A writable cgroup namespace (an isolated subtree — NOT the host tree)
#   • CAP_SYS_ADMIN  — required for mounting the cgroup hierarchy and
#                      managing kernel namespaces
#   • CAP_SYS_PTRACE — used by systemd-journald to read process metadata
#   • Full /sys/fs/cgroup access so systemd can create per-service slices
#
# A fine-grained capability set + custom seccomp profile can replace
# --privileged in hardened production environments, but --privileged is the
# correct, self-documenting choice for a developer/CI test harness where
# reproducibility matters more than minimal attack surface.
#
# NO HOST-STATE LEAKAGE
# ─────────────────────────────────────────────────────────────────────────────
# This image does NOT bind-mount /sys/fs/cgroup or any other host directory.
# --privileged creates an *isolated* cgroup namespace for the container; the
# host cgroup tree is not visible to or modified by the container.  Each
# `docker run` starts from a clean, immutable image layer with no persistent
# state.
#
# USAGE
# ─────────────────────────────────────────────────────────────────────────────
#   docker build -t cistern-systemd-test .
#   docker run --privileged --rm -d --name systemd-test cistern-systemd-test
#   docker exec systemd-test systemctl is-system-running   # → running|degraded
#   docker stop systemd-test

FROM debian:bookworm

# ENV container=docker is read by systemd to enable container-specific unit
# overrides and to skip hardware-only targets (kmod-static-nodes, cryptsetup,
# etc.) that would fail or hang indefinitely inside a container.
ENV container=docker

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        systemd \
        systemd-sysv \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Mask units that are incompatible with a container environment.
# These units require direct hardware access, VT consoles, or kernel
# interfaces that are unavailable inside Docker; leaving them enabled causes
# systemd to report 'failed' units on every boot.
#
# Masking = symlink to /dev/null in /etc/systemd/system/.  This takes
# precedence over unit files anywhere else on the search path and survives
# systemd package upgrades that would otherwise restore the files.
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

# Docker sends SIGTERM to PID 1 by default.  systemd's clean-shutdown signal
# is SIGRTMIN+3.  Override the stop signal so `docker stop` triggers an
# orderly unit shutdown rather than an abrupt kill.
STOPSIGNAL SIGRTMIN+3

# systemd must run as PID 1.  /lib/systemd/systemd is the canonical path on
# Debian; /sbin/init is a symlink to the same binary and is equally valid.
CMD ["/lib/systemd/systemd"]

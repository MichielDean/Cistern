#!/usr/bin/env bash
# start-castellarius.sh — thin wrapper around `ct castellarius start`.
# Used as the ExecStart target in the systemd service unit.
#
# The opencode agent reads credentials from the environment or its own config.
set -euo pipefail

exec ct castellarius start

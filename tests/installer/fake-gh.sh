#!/usr/bin/env bash
# fake-gh — stub for the GitHub CLI used in Cistern installer integration tests.
#
# ct doctor checks "gh CLI installed" and "gh authenticated"; this stub
# satisfies both by accepting any arguments and exiting 0 silently.
# Placed at /usr/local/bin/gh so exec.LookPath("gh") resolves it.
case "${1:-}" in
    auth) exit 0 ;;
    *) exit 0 ;;
esac

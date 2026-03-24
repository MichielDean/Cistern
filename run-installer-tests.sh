#!/usr/bin/env bash
# run-installer-tests.sh — installer test harness for Cistern
#
# Builds the installer-test Docker image (systemd + ct + fakeagent claude
# stub), starts a container, runs scaffolding tests, and reports results in
# GitHub Actions annotation format.
#
# Usage:
#   ./run-installer-tests.sh
#
# Exit codes:
#   0  all tests passed
#   1  one or more tests failed, or setup failed
#
# Requirements:
#   docker (with BuildKit support)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# shellcheck source=test/installer/helpers.sh
source "${SCRIPT_DIR}/test/installer/helpers.sh"

# ─── Test cases ───────────────────────────────────────────────────────────────

# test_systemd_boots verifies that systemd reaches a stable operating state
# inside the container.
# Acceptable states: running (all units OK), degraded (some non-essential
# units failed — still functional for installer tests).
test_systemd_boots() {
    local status
    status=$(docker exec "${CONTAINER_NAME}" \
        systemctl is-system-running 2>/dev/null || echo "error")
    case "${status}" in
        running|degraded) return 0 ;;
        *) return 1 ;;
    esac
}

# test_ct_available verifies that the ct binary is present and executable.
test_ct_available() {
    exec_in_container ct version >/dev/null
}

# test_fakeagent_available verifies that fakeagent is in the PATH and that
# the /usr/local/bin/claude symlink resolves correctly.
test_fakeagent_available() {
    exec_in_container which fakeagent >/dev/null &&
    exec_in_container which claude    >/dev/null
}

# test_ct_init verifies that `ct init` exits 0 and creates the Cistern config
# file at the expected location.
test_ct_init() {
    if ! exec_in_container ct init >/dev/null 2>&1; then
        return 1
    fi
    exec_in_container test -f /root/.cistern/cistern.yaml
}

# test_ct_doctor verifies that `ct doctor` terminates without crashing.
# It is expected to exit 1 (some checks fail in the minimal container
# environment — e.g., gh CLI not installed), but it must not exit with a
# signal or an unexpected code ≥ 2.
test_ct_doctor() {
    local exit_code=0
    exec_in_container ct doctor >/dev/null 2>&1 || exit_code=$?
    [[ "${exit_code}" -le 1 ]]
}

# test_service_status_helper verifies that the service_status helper function
# returns a non-empty string when querying a systemd unit that is not
# installed.  The expected result is "inactive" (not found = inactive).
test_service_status_helper() {
    local status
    status=$(service_status "cistern-castellarius.service")
    [[ -n "${status}" ]]
}

# ─── Runner ───────────────────────────────────────────────────────────────────

# run_test calls a test function and records pass/fail.
# Using an `if` statement means set -e does not trigger on a non-zero return.
run_test() {
    local name="$1"
    local func="$2"
    if "${func}"; then
        pass "${name}"
    else
        fail "${name}"
    fi
}

# ─── Cleanup ──────────────────────────────────────────────────────────────────

cleanup() {
    teardown_container
}
trap cleanup EXIT

# ─── Main ─────────────────────────────────────────────────────────────────────

main() {
    require_docker
    setup_container "${SCRIPT_DIR}"

    run_test "systemd boots in container"            test_systemd_boots
    run_test "ct binary is available"                test_ct_available
    run_test "fakeagent available as claude stub"    test_fakeagent_available
    run_test "ct init creates cistern config"        test_ct_init
    run_test "ct doctor runs without crash"          test_ct_doctor
    run_test "service_status helper queries systemd" test_service_status_helper

    printf '\nResults: %d passed, %d failed\n' "${PASS_COUNT}" "${FAIL_COUNT}"

    [[ "${FAIL_COUNT}" -eq 0 ]]
}

main "$@"

# Context

## Item: ci-47i5f

**Title:** Integration tests: session name isolation and production opt-out
**Status:** in_progress
**Priority:** 2

### Description

The Castellarius integration tests run on the same machine as the live production Castellarius (lobsterdog self-hosted runner). This creates two risks:

1. Tmux session name collision: tests create sessions named myrepo-worker-alpha. If a production aqueduct ever used a repo named myrepo with a worker named alpha, the heartbeat zombie check could kill a live production session. Fix: prefix integration test session names with a short hash derived from t.TempDir() or t.Name() so they cannot collide with production (e.g. it3a7f-myrepo-worker-alpha).

2. No opt-out for production machines: there is no way to skip integration tests on a production machine without modifying test code. Fix: add a CISTERN_SKIP_INTEGRATION=1 env var check in checkIntegrationPrereqs (alongside the existing tmux and ct binary checks). The lobsterdog machine can set this in its runner environment if desired; CI leaves it unset.

Changes required:
1. In integrationRunner.Spawn, derive a short prefix from t.TempDir() (first 6 hex chars of the basename hash) and prepend it to the sessionID
2. Update isTmuxAlive checks and cleanup to match the prefixed session name
3. In checkIntegrationPrereqs, add: if os.Getenv("CISTERN_SKIP_INTEGRATION") != "" { t.Skip(...) }

Acceptance criteria:
- Integration test tmux sessions are named with a unique prefix that cannot match any production session
- CISTERN_SKIP_INTEGRATION=1 causes all four integration tests to skip cleanly
- Tests still pass on CI (no CISTERN_SKIP_INTEGRATION set)

## Current Step: docs

- **Type:** agent
- **Role:** docs_writer
- **Context:** full_codebase

## ⚠️ REVISION REQUIRED — Fix these issues before anything else

This droplet was recirculated. The following issues were found and **must** be fixed.
Do not proceed to implementation until you have read and understood each issue.

### Issue 1 (from: reviewer)

♻ 1 finding. The session prefix added to integrationRunner.Spawn (line 124) is invisible to the Castellarius heartbeat (scheduler.go:1424), which constructs session names as repo+'-'+assignee without the prefix. This causes isTmuxAlive to always return false for test sessions, meaning TestIntegration_HeartbeatRecovery passes coincidentally (name mismatch) rather than because the heartbeat actually detected a dead session. Fix: move the prefix into the repo name in intConfig() so both the runner and heartbeat derive matching session names.

### Issue 2 (from: reviewer)

♻ 1 finding. sessionPrefix (line 64) always returns the same value across all tests because filepath.Base strips the unique parent path from t.TempDir(), leaving only the sequential counter ('003'). All four tests get prefix '88c041'. Fix: hash the full t.TempDir() path or use t.Name(). Prior issue ci-47i5f-s7oq5 (session name mismatch) is resolved — prefix is correctly embedded in repo name via intConfig(prefix).

### Issue 3 (from: reviewer)

No findings. Prior issue ci-47i5f-smkm8 resolved: sessionPrefix now hashes full t.TempDir() path (no filepath.Base). Fresh review: session name contract verified against scheduler.go:988 and :1424 — both sides compute repo.Name+"-"+assignee, prefix correctly embedded in repo name via intConfig. CISTERN_SKIP_INTEGRATION check correct. CT_BIN forwarding clean. All four tests use consistent prefix+"-myrepo" repo name. No new issues.

### Issue 4 (from: qa)

All four integration tests pass. CISTERN_SKIP_INTEGRATION=1 skips all four cleanly. Session names are prefixed (e.g. ece918-myrepo-worker-alpha) via repo name embedding — heartbeat correctly matches. Full test suite green.

### Issue 5 (from: security)

No security issues found. Diff touches: (1) test infrastructure — session prefix via SHA-256 of TempDir, CT_BIN env for source-built ct in fakeagent (test-only binary), CISTERN_SKIP_INTEGRATION opt-out; (2) production — CT_DB env var in resolveDBPath (mirrors existing --db flag), gh auth check made informational in ct doctor (delivery step still enforces auth). All input flows traced: fakeagent droplet ID uses exec.Command discrete args (no shell injection), intShellQuote uses proper escaping with test-controlled inputs, CT_BIN only writable by test harness. No injection, auth bypass, secrets exposure, or resource safety issues.

---

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>cistern-git</name>
    <description>Each droplet has an isolated worktree at `~/.cistern/sandboxes/&lt;repo&gt;/&lt;droplet-id&gt;/`.</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-git/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-47i5f

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-47i5f
    ct droplet recirculate ci-47i5f --to implement

**Pool (cannot currently proceed):**
    ct droplet pool ci-47i5f

Add notes before signaling:
    ct droplet note ci-47i5f "What you did / found"

The `ct` binary is on your PATH.

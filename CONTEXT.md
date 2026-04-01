# Context

## Item: ci-1zeaa

**Title:** Scheduler: auto-route to reviewer when implementer loops on unresolved reviewer issue
**Status:** in_progress
**Priority:** 2

### Description

When a reviewer opens an issue on a droplet, only the reviewer cataractae can close it. If the implementer fixes the issue and recirculates back to itself (because implement has no on_recirculate route to reviewer), the droplet is permanently stuck: implementer cannot signal pass while the issue is open, and the reviewer never runs.

Fix: in the recirculate handler, after determining the cataractae and the open issues for the droplet, check if:
  (a) the current cataractae is 'implement' (or similar non-reviewer stage)
  (b) there is at least one open issue opened by a reviewer cataractae
  (c) the same reviewer issue has appeared in the last N consecutive recirculate notes (detect loop, suggested N=2)

When all three conditions are met, instead of recirculating back to implement, route the droplet directly to the reviewer cataractae so it can verify the fix and close the issue. Write a structured note: '[scheduler:loop-recovery] detected implement→implement loop on reviewer issue <issue-id> — routing to reviewer'.

This removes the deadlock without human intervention. The reviewer runs, sees the implementer's fix, closes the issue, and the droplet advances normally.

Acceptance criteria: a droplet in the implement→implement loop due to an open reviewer issue is automatically routed to reviewer within 2 recirculate cycles; the reviewer can close the issue and the droplet advances; structured recovery note is written; no human intervention required.

## Current Step: docs

- **Type:** agent
- **Role:** docs_writer
- **Context:** full_codebase

## ⚠️ REVISION REQUIRED — Fix these issues before anything else

This droplet was recirculated. The following issues were found and **must** be fixed.
Do not proceed to implementation until you have read and understood each issue.

### Issue 1 (from: security)

No security issues found. Audited all changed files in the diff:

**Scheduler loop recovery (scheduler.go:850-900):**
- Routing target (issue.FlaggedBy) is validated against the workflow definition via lookupCataracta() before use — prevents routing to arbitrary/non-existent steps.
- ListIssues uses parameterized SQL queries (client.go:1166-1177) — no injection.
- loopRecoveryPendingCount uses a trailing-space-terminated marker for string matching — no prefix collision between issue IDs (e.g. iss-1 vs iss-10).
- Error paths (ListIssues failure, GetNotes failure) fail closed — fall through to normal routing with warn log, no insecure fallback.

**Dashboard web (dashboard_web.go):**
- Ring buffer replaced with frame-based lastFrame approach: shared state protected by mutex, generation counter prevents stale timer corruption, bytes.Clone prevents aliasing.
- UnassignedItems JSON serialization uses json.NewEncoder which properly escapes strings — no XSS in API response.
- WebSocket sends PTY output from a controlled child process — no user-input injection vector.

**Dashboard TUI (dashboard.go, dashboard_tui.go):**
- Dynamic colW computed from terminal width (not user-controlled input). Minimum floor of 9 enforced.
- Unassigned items rendered in TUI context from database fields — no web injection surface.

No blocking, required, or suggestion-level findings.

---

## Recent Step Notes

### From: security

No security issues found. Audited all changed files in the diff:

**Scheduler loop recovery (scheduler.go:850-900):**
- Routing target (issue.FlaggedBy) is validated against the workflow definition via lookupCataracta() before use — prevents routing to arbitrary/non-existent steps.
- ListIssues uses parameterized SQL queries (client.go:1166-1177) — no injection.
- loopRecoveryPendingCount uses a trailing-space-terminated marker for string matching — no prefix collision between issue IDs (e.g. iss-1 vs iss-10).
- Error paths (ListIssues failure, GetNotes failure) fail closed — fall through to normal routing with warn log, no insecure fallback.

**Dashboard web (dashboard_web.go):**
- Ring buffer replaced with frame-based lastFrame approach: shared state protected by mutex, generation counter prevents stale timer corruption, bytes.Clone prevents aliasing.
- UnassignedItems JSON serialization uses json.NewEncoder which properly escapes strings — no XSS in API response.
- WebSocket sends PTY output from a controlled child process — no user-input injection vector.

**Dashboard TUI (dashboard.go, dashboard_tui.go):**
- Dynamic colW computed from terminal width (not user-controlled input). Minimum floor of 9 enforced.
- Unassigned items rendered in TUI context from database fields — no web injection surface.

No blocking, required, or suggestion-level findings.

### From: qa

All tests pass (10 packages). No open QA issues. Phase 1: no prior QA issues; prior reviewer issues all confirmed resolved. Phase 2: implementation correct — loop recovery fires on ResultRecirculate&&next==step.Name, loopDetectN=2 math correct, both error paths handled with warn+fallback, prefix-match fix in place. 7 tests with descriptive names cover all key paths: no issues, first-cycle pending note, second-cycle routing, structured note format, GetNotes error, ListIssues error, closed issue non-trigger. All acceptance criteria met.

### From: reviewer

No findings.

### From: reviewer

Phase 1: resolved both prior issues (ci-1zeaa-phe3g prefix-match fix confirmed, ci-1zeaa-wrcf8 ListIssues error test confirmed). Phase 2: full adversarial review — no new findings. Security, logic, error handling, interface contracts, resource management, and test coverage all clean.

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>cistern-git</name>
    <description>---</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-git/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-1zeaa

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-1zeaa
    ct droplet recirculate ci-1zeaa --to implement

**Pool (cannot currently proceed):**
    ct droplet pool ci-1zeaa

Add notes before signaling:
    ct droplet note ci-1zeaa "What you did / found"

The `ct` binary is on your PATH.

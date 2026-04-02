# Context

## Item: ci-z3jf8

**Title:** TUI cockpit: Status panel with live-refresh
**Status:** in_progress
**Priority:** 2

### Description

Read-only panel registered as module 3 (key: 3) rendering ct status output: cistern counts, aqueduct flow summary, castellarius health. Auto-refreshes on a 5-second ticker with idle backoff following dashboardTUIModel pattern. r key force-refreshes. Acceptance: pressing 3 shows current system status; data refreshes automatically; r triggers immediate refresh.

## Current Step: docs

- **Type:** agent
- **Role:** docs_writer
- **Context:** full_codebase

## ⚠️ REVISION REQUIRED — Fix these issues before anything else

This droplet was recirculated. The following issues were found and **must** be fixed.
Do not proceed to implementation until you have read and understood each issue.

### Issue 1 (from: security)

No security issues found. Diff adds a read-only local TUI panel (status panel, module 3) with no network-facing surface. User keyboard input controls only scroll position and a refresh trigger — no user input reaches queries, file paths, or shell calls. fetchDashboardData reads local SQLite (pre-existing, unchanged). Scroll clamping is correct. Refresh loop is rate-limited (5s/30s idle). Dashboard web changes (attach/resize repaint-marker) are internal PTY rendering fixes. Deletion of audit.go reduces attack surface.

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
    ct droplet pass ci-z3jf8

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-z3jf8
    ct droplet recirculate ci-z3jf8 --to implement

**Pool (cannot currently proceed):**
    ct droplet pool ci-z3jf8

Add notes before signaling:
    ct droplet note ci-z3jf8 "What you did / found"

The `ct` binary is on your PATH.

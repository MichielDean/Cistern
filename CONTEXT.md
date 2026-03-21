# Context

## Item: ci-0vm8f

**Title:** Cataractae peek: read-only live observer for active aqueduct sessions
**Status:** in_progress
**Priority:** 2

### Description

Add ability to observe any active cataractae session in real-time without interacting with it. Requirements:
- GET /api/aqueducts/{name}/peek returns current tmux pane content as text
- WebSocket endpoint /ws/aqueducts/{name}/peek streams live pane output (poll tmux every 500ms, send diffs)
- Web UI: clicking an active aqueduct arch opens a peek panel/modal showing live session output
- Read-only: no keyboard input forwarded, no interaction possible, purely observational
- Shows last N lines of pane (configurable, default 100)
- Auto-scrolls to bottom, toggle to pin scroll position
- Clear label: 'Observing — read only'
- Falls back gracefully if aqueduct is idle or tmux session not found

## Current Step: simplify

- **Type:** agent
- **Role:** simplifier
- **Context:** full_codebase

## Recent Step Notes

### From: manual

Simplified: (1) extracted parsePeekLines() helper to eliminate duplicated ?lines= query parsing in GET and WS peek handlers, (2) consolidated two separate session-not-active guard clauses into one in the GET peek handler. Net -22 lines. Tests: all 9 packages pass.

### From: manual

Phase 2: (1) dashboard_web.go render template — XSS via esc() in onclick context: esc() converts single-quote to &#39; but the HTML attribute parser decodes &#39; back to ' before the JS engine sees it, so an aqueduct name like a'); alert(1)// produces peekOpen('a'); alert(1)//) — stored XSS exploitable by anyone with pipeline-config write access. Fix: use a data-aqname attribute and addEventListener instead of an inline onclick string. (2) dashboard_web.go WS handler — for-range-ticker.C success path has zero test coverage: TestWsPeek_NonWebSocketRejected and TestWsPeek_MissingKeyRejected only test rejection branches (426/400); the entire streaming loop (lookupAqueductSession → HasSession → Capture → computeDiff → wsSendText) is untested for a connected client.

### From: manual

Fixed XSS in dashboard_web.go: replaced inline onclick="peekOpen('...')" with data-aqname attribute + delegated addEventListener on app element. esc() encodes " as &quot; (safe in double-quoted attribute) while browsers decode &#39; back to ' before JS execution, making the old approach exploitable. Added TestWsPeek_SuccessfulStreamIdle and TestWsPeek_SuccessfulStreamActive covering the full WS success loop (wsUpgrade → ticker → lookupAqueductSession → HasSession → Capture → computeDiff → wsSendText) using httptest.NewServer + real net.Dial + manual RFC6455 frame decoding. Added readWSTextFrame helper. All 9 packages pass. Committed fde6069.

### From: manual

Fixed XSS (onclick→data-aqname+addEventListener) and added TestWsPeek_SuccessfulStreamIdle/Active covering full WS streaming loop. All 9 packages pass.

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>.claude/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>code-simplifier</name>
    <description>code-simplifier</description>
    <location>.claude/skills/code-simplifier/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-0vm8f

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-0vm8f
    ct droplet recirculate ci-0vm8f --to implement

**Block (genuinely blocked, cannot proceed):**
    ct droplet block ci-0vm8f

Add notes before signaling:
    ct droplet note ci-0vm8f "What you did / found"

The `ct` binary is on your PATH.

# Context

## Item: ci-zcb8j

**Title:** Peek always live-attaches regardless of TMUX env var
**Status:** in_progress
**Priority:** 2

### Description

The dashboard TUI's peek feature is broken when accessed via the ttyd web dashboard. openPeekOn() branches on insideTmux() — which checks $TMUX — but ttyd runs ct dashboard as a fresh process with no $TMUX set. This falls through to openInlinePeek(), which displays a useless 'not inside tmux — for live view: tmux attach-session -t X -r' hint as a header, and the content area shows nothing useful.

Fix: remove the insideTmux() guard from openPeekOn(). Always attempt tmux attach-session -r directly. The tmuxNewWindowFunc path (opening a new tmux window) was only useful when already inside tmux and wanting a separate window — make that the fallback only when explicitly desired, not the default. The inline capture-pane overlay (--snapshot) remains available for non-interactive contexts.

Specifically:
- In dashboard_tui.go: openPeekOn() should always call tmuxAttachFunc directly (like the non-tmux path does), not branch on insideTmux()
- The 'not inside tmux' message in openInlinePeek() header should be removed; that path is now only reached when tmuxAttachFunc fails
- insideTmux() and tmuxNewWindowFunc can be removed if they have no other callers after this change

Acceptance criteria:
- Pressing 'p' on the web dashboard attaches read-only to the active agent's tmux session
- All existing peek tests pass
- go test ./...

## Current Step: delivery

- **Type:** agent
- **Role:** delivery

## ⚠️ REVISION REQUIRED — Fix these issues before anything else

This droplet was recirculated. The following issues were found and **must** be fixed.
Do not proceed to implementation until you have read and understood each issue.

### Issue 1 (from: security)

No security issues found. Session name derives from trusted config file (not user input); exec.Command with argument array precludes shell injection; -r flag correctly enforces read-only attach. Removal of insideTmux/tmuxNewWindowFunc reduces code surface. No new network endpoints introduced.

---

## Recent Step Notes

Sandbox corruption: CONTEXT.md describes ci-cv5jf (Remove ct audit command) but branch contains ci-zcb8j commits (Peek always live-attaches). Data inconsistency prevents delivery — requires sandbox reset.

<available_skills>
  <skill>
    <name>cistern-github</name>
    <description>Use `gh` CLI for all GitHub operations. Prefer CLI over GitHub MCP servers for lower context usage.</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-github/SKILL.md</location>
  </skill>
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
    ct droplet pass ci-zcb8j

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-zcb8j
    ct droplet recirculate ci-zcb8j --to implement

**Pool (cannot currently proceed):**
    ct droplet pool ci-zcb8j

Add notes before signaling:
    ct droplet note ci-zcb8j "What you did / found"

The `ct` binary is on your PATH.

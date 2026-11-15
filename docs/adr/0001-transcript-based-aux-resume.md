# ADR 0001: Transcript-based resume for auxiliary agents (no opencode `-s`)

Date: 2026-11-14
Status: Accepted

## Context

Auxiliary agents (filter, triage, wizard) run as project-local opencode
agents (`.opencode/agents/<name>.md`). Resuming an opencode session with
`run -s <session-id>` re-resolves the agent from the current project state.
Across server restarts or fresh `-s` invocations this fails with
"Agent not found", even when the working directory still contains the agent
files (observed via cistern e894f63 mechanism). The resume seam silently
couples cistern session continuity to opencode's in-process server state.

## Decision

Drop the `-s` resume seam entirely.

- Every aux-agent invocation is a **fresh** opencode session
  (`buildAgentRunCommand` never emits a resume or continue flag).
- Session chaining lives in **cistern's DB**, not opencode's server:
  each round appends its user/assistant exchange to the persisted
  `filter_sessions.messages` transcript.
- Resume = load the persisted transcript and replay it into the fresh
  session's prompt via `buildResumePrompt`: a `PRIOR CONVERSATION` block
  followed by the new message, so the agent sees full chronology.
- `--resume` on `ct filter`, `ct triage`, `ct wizard`, and the dashboard
  resume endpoint now takes the **cistern session ID** (printed to stderr by
  the prior round) — not an opencode session ID.
- `filter_sessions` gained an `agent` discriminator column (migration 021,
  default `'filter'`) so triage and wizard sessions can persist rounds to the
  same table; non-filter rows are hidden from `ListFilterSessions`.
- The dashboard TUI keeps its in-memory transcript and builds follow-up
  prompts the same way via `resumePromptFromHistory` → `buildResumePrompt`;
  the stored `llm_session_id` is per-turn provenance and is never a resume key.

## Consequences

- Resume works across restarts and directory changes: the only durable
  state is cistern's SQLite DB.
- Resume prompts grow linearly with round count (full transcript is
  re-sent each round). Acceptable at filtration-round scales.
- opencode's own conversation memory is unused for aux agents; model
  fidelity of follow-ups depends on transcript completeness, which the
  persistence layer guarantees.

---
name: mp-handoff
description: Compact the current cataractae session into a handoff document for the next cataractae to pick up — recirculation, delivery fixes, multi-stage work.
disable-model-invocation: true
---
Write a handoff document (`HANDOFF.md` in the worktree root, transient — never commit it) summarising the current session so the next cataractae can continue the work. This is the recirculate payload: the same context that would otherwise live only in your head.

Include:

- **State**: what stage the work reached, and what the next session must know to continue it
- **Suggested skills**: which skills the next cataractae should consult and why
- **Open threads**: unresolved findings, failing checks, questions

Do not duplicate content already captured in other artifacts (CONTEXT.md, DESIGN_BRIEF.md, commits, diffs). Reference them by path instead.

Redact any sensitive information, such as API keys, passwords, or personally identifiable information.

Tailor the document to the destination stage when the recirculation path is known (e.g. `on_recirculate: implement`).

---

## Attribution

Adapted for Cistern from Matt Pocock's [handoff](https://github.com/mattpocock/skills) skill
(v1.3.2, MIT, Copyright (c) 2026 Matt Pocock). Technique content adapted to cataractae
context; upstream keeps the canonical version. See `internal/skills/ATTRIBUTION.md`.

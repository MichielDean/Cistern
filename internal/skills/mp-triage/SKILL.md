---
name: mp-triage
description: Move pooled, failed, or rough droplets through a triage state machine — categorise, verify the claim, grill if needed, and write agent-ready briefs. Used by the ct triage entrypoint.
---

# Triage

Move work units through a small state machine of triage roles so that everything
entering the pipeline has been categorised, verified, and — when it comes back —
briefed enough to implement AFK.

## What gets triaged

1. **Pooled droplets** — a cataractae signalled `pool` (cannot currently proceed).
2. **Failed droplets** — recirculated repeatedly or blocked.
3. **Rough ideas** — raw work the user wants checked before it enters the pipeline.

Every triaged droplet carries exactly one category and one state.

## Categories

Two categories:

- `bug`: something that exists is broken
- `enhancement`: new capability or improvement

## States

Five states, tracked as tags in the droplet description (`[triage:<state>]`):

- `needs-triage`: evaluation in progress
- `needs-info`: waiting on the reporter for more information
- `ready-for-agent`: fully specified — the description reads as an agent brief; the pipeline can implement it AFK with `ct droplet add` + `--depends-on` wiring
- `ready-for-human`: needs human implementation (judgment calls, external access, design decisions, manual testing)
- `wontfix`: will not be actioned — cancel with `ct droplet cancel` and the reason in a note

State transitions: an untriaged droplet normally goes to `needs-triage` first; from
there to `needs-info`, `ready-for-agent`, `ready-for-human`, or `wontfix`. `needs-info`
returns to `needs-triage` once the reporter replies (new note activity). The user can
override at any time; flag transitions that look unusual and ask before proceeding.

## Show what needs attention

Query the cistern and present three buckets, oldest first:

1. **Untriaged**: pooled/failed droplets with no `[triage:]` tag.
2. **`needs-triage`**: evaluation in progress.
3. **`needs-info` with new note activity since the last triage note**: re-evaluate.

Show counts and a one-line summary per item (`ct droplet list`. Let the user pick.

## Triage a specific droplet

1. **Gather context.** Read the droplet fully (`ct droplet show <id>`), plus its history
   (`ct droplet log <id>`) and any prior triage notes so you don't re-ask resolved
   questions. Explore the codebase for the owning repo; use the project's glossary.
   Run two checks against the codebase: (a) **redundancy** — search for an existing
   implementation of the requested behaviour by domain concept (not just the request's
   wording), and state where you looked; (b) **prior rejection** — check for similar
   cancelled droplets (`ct droplet list --status cancelled`).

2. **Recommend.** Give your category and state recommendation with reasoning, plus a
   brief codebase summary (including whether it is already implemented). Wait for direction.

3. **Verify the claim.** Before any grilling, check the claim holds. For a bug,
   reproduce it. For a behaviour request, confirm the current behaviour by running the
   relevant command. Report what happened: confirmed (with code path), failed, or
   insufficient detail (a strong `needs-info` signal). A confirmed verification makes a
   much stronger agent brief.

4. **Grill (if needed).** If the description needs fleshing out, grill it into shape a
   round of questions at a time (mp-grilling methodology): sharpen domain terms against
   `GLOSSARY.md` and record decisions as they land. Update the droplet with
   `ct droplet edit`.

5. **Apply the outcome:**
   - `ready-for-agent`: rewrite the droplet description as an agent brief (format below),
     and wire dependencies with `ct droplet deps`. If the pipeline work should enter as new
     droplets split from the original, file them with `ct droplet add --depends-on`.
   - `ready-for-human`: same brief structure, plus a note on why it can't be delegated
     (judgment calls, external access, design decisions, manual testing).
   - `needs-info`: add triage notes to the droplet (`ct droplet note`).
   - `wontfix`: `ct droplet cancel <id>` with a note giving the reason.

## Agent brief format

For `ready-for-agent`, the droplet description must contain:

```markdown
**Category:** bug | enhancement
**State:** ready-for-agent
**Verified:** <what you reproduced or confirmed, with the command and result>

**Goal:** one sentence.

**Acceptance criteria:**
- <observable, verifiable criterion 1>
- <criterion 2>

**Constraints:**
- files/areas in scope: <paths>
- out of scope: <paths or behaviours>

**Context:**
<what the implementing cataractae needs to know that the codebase won't tell it —
prior attempts that failed, invariants, related droplets>
```

Every criterion must be verifiable by a command, file path, or test — otherwise it has
not been sharpened enough to leave `needs-triage`.

## Needs-info template

```markdown
[triage:needs-info]

**What we've established so far:**

- point 1
- point 2

**What we still need from you:**

- question 1
- question 2
```

Capture everything resolved during grilling under "established so far" so the work
isn't lost. Questions must be specific and actionable, not "please provide more info".

## Resuming a previous session

If prior triage notes exist on the droplet, read them, check whether any outstanding
questions have been answered in later notes, and present an updated picture before
continuing. Don't re-ask resolved questions.

---

## Attribution

State machine adapted for Cistern from Matt Pocock's [triage](https://github.com/mattpocock/skills) skill
(v1.3.2, MIT, Copyright (c) 2026 Matt Pocock); issue-tracker mechanics remapped to droplets and the ct CLI.
See `internal/skills/ATTRIBUTION.md`.

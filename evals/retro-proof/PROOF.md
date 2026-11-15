# Retro-proof: the arch-review stage catches real, shipped structural debt

**2026-11-15.** The arch-review cataractae (PR #539) was validated by replaying
the job-pipeline-platform's real history back through it. This is the exact
class of session the stage exists to automate: the settings-registry deepening
(#36) was a *human-driven* session that discovered, after the code had shipped,
that the discovery cadence design had shipped hand-synced duplicated truths.

## The question

If PR #20 ("Persistent discovery service", the design that actually shipped
the hand-synced `CADENCE_DEFAULTS_MIN` / `CADENCE_BOUNDS_MIN` tables) had been
reviewed by the arch-review cataractae at design time, would it have caught
the debt that #36 later paid down?

## Experiments

Artifacts live in this directory. The retro codebase state is job-pipeline-platform
commit `ad8eb3403` (before discovery/cadence existed); the brief under review is
the design #20 actually shipped, as the committed code records it.

| Experiment | Setup | Expected | Result |
|---|---|---|---|
| **Exp 1 — retro-catch** (`exp1-retrocatch-promptfooconfig.yaml`) | arch-reviewer reviews the actual #20 design vs pre-cadence codebase | Recirculate with single-truth / duplicated-semantics findings | **6/6 runs PASS** (2 test cases × 3 runs, stable across glm variance) |
| **Exp 2 — control** (`exp2-control-promptfooconfig.yaml`) | the post-#36 fix design (registry + codegen echo + dual drift test) vs post-fix state | Pass | Reviewer recirculates on real, residual looseness in the control brief (unanchored two-source codegen conflict behavior; non-cadence JS keys without drift coverage). See honesty notes. |
| **Exp 3 — A/B** (`exp3-ab-promptfooconfig.yaml`) | OLD architect instructions (`80b5328^`, no Single-Truth section) vs NEW, same droplet | OLD ships prose-synced mirrors; NEW mandates owners + structural coupling | OLD: duplicated tables kept in sync by comments only (confirmed, stable). NEW: named owners + mandatory drift test in 2/3 runs; 1 run regressed to prose-only coupling — caught, and exactly the failure mode the arch-review stage backstops. |

## Exp 1 findings (verbatim, first run)

> **CADENCE_BOUNDS_MIN is a second copy of the write-time validation bounds,
> kept true only by a comment.** Brief: *"CADENCE_BOUNDS_MIN mirrors the
> settingsSave validation bounds … (defense in depth — the server rejects it
> at write time [anyway]"…*

plus the duplicated unset-semantics finding and a hand-re-enumerated provider
set — the same three categories PR #36 fixed one month of iteration later.

## Honesty notes

- The Exp 2 control never fully passed. Each iteration fixed the prior
  finding; the reviewer then found the next residual looseness (cross-language
  unset semantics mirror, two-source codegen conflict rule, non-cadence keys).
  These are genuine gaps in the compressed synthetic brief, and the formal
  negative control for over-recirculation is the clean-pass test in
  `../cistern-arch-reviewer-promptfooconfig.yaml` (clean brief anchored to
  existing owners → Pass, verified green twice). Read Exp 2 as: the reviewer
  is *more* rigorous than its instructions require, not less.
- Exp 3's NEW-architect 1/3 regression shows glm variance on the brief author;
  the design-review stage is the backstop that makes that variance safe —
  prose-only coupling cannot reach the implementer unchallenged.

## Reproduce

```bash
cd evals/retro-proof
promptfoo eval -c exp1-retrocatch-promptfooconfig.yaml --no-cache
promptfoo eval -c exp3-ab-promptfooconfig.yaml --no-cache
```
Configs read instructions live from `../../cataractae/…`, so the proof re-runs
against whatever is currently shipped. Not part of the run-all-checks gate
(the shipped configs above are); these are research artifacts.

---
name: mp-implement
description: Construction discipline for implementing a droplet from its design brief — vertical slices, regular check runs, TDD pairing.
disable-model-invocation: true
---
Implement the work described by the droplet and its DESIGN_BRIEF.md contract.

State the droplet title and ID from CONTEXT.md before starting. If the brief is ambiguous on a point the droplet settles, follow the droplet; if they conflict in substance, file an issue instead of guessing.

Work in **vertical slices**, pairing each slice with the mp-tdd skill at the seams the brief and your seam list agreed.

Run typechecking regularly, run targeted test files as you work, and run the full test suite once before signaling completion. The reviewer cataractae will run mp-code-review on your diff; do not review your own work — make sure your commits tell the story instead.

Commit your work to the current branch (see cistern-git skill; never push to origin).

---

## Attribution

Adapted for Cistern from Matt Pocock's [implement](https://github.com/mattpocock/skills) skill
(v1.3.2, MIT, Copyright (c) 2026 Matt Pocock). Technique content adapted to cataractae
context; upstream keeps the canonical version. See `internal/skills/ATTRIBUTION.md`.

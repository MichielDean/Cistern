---
name: mp-code-review
description: Two-axis review of a diff — Standards (does the code follow the repo's documented coding standards?) and Spec (does the code match what the originating droplet asked for?). Cataractae complement to the reviewer identity's contract analysis.
---

Review the diff between `HEAD` and the point the implementation started at.

- **Standards**: does the code conform to this repo's documented coding standards?
- **Spec**: does the code faithfully implement the originating droplet?

A cataractae is a single session — the two axes run as **two sequential passes** in one context, kept logically separate and reported side by side. Never merge findings from the two passes.

## Process

### 1. Pin the fixed point

The base commit is where the implementer's branch diverged from origin/main. Find it in CONTEXT.md or via the worktree's merge-base (`git merge-base HEAD origin/main`). If it cannot be determined, review the full diff against origin/main.

Capture the diff command once: `git diff <base>...HEAD` (three-dot, so the comparison is against the merge-base). Also note the list of commits via `git log <base>..HEAD --oneline`.

Before going further, confirm the fixed point resolves (`git rev-parse <base>`) and the diff is non-empty. A bad ref or empty diff is a signal-pass situation, not a review.

### 2. Identify the spec source

For a Cistern droplet the spec sources are, in order:

1. The droplet body and revision notes, as recorded in CONTEXT.md.
2. DESIGN_BRIEF.md, if the architect produced one (it is the implementer's contract).
3. The original droplet as printed by `ct droplet show <id>` (the ID is in CONTEXT.md).
4. If none are available, the Spec pass skips and reports "no spec available".

### 3. Identify the standards sources

Search the repo for every file that documents how code should be written. When `CONTRIBUTING.md` exists, it must be on the list. The cataractae AGENTS.md instructions for this repo's conventions also count.

On top of whatever the repo documents, the Standards axis always carries the **smell baseline** below: a fixed set of Fowler code smells (_Refactoring_, ch.3) that applies even when a repo documents nothing. Two rules bind it:

- **The repo overrides.** A documented repo standard always wins; where it endorses something the baseline would flag, suppress the smell.
- **Always a judgement call.** Each smell is a labelled heuristic ("possible Feature Envy"), never a hard violation. Like any standard here, skip anything tooling already enforces.

Each smell reads *what it is* → *how to fix*; match it against the diff:

- **Mysterious Name**: a function, variable, or type whose name doesn't reveal what it does or holds. → rename it; if no honest name comes, the design's murky.
- **Duplicated Code**: the same logic shape appears in more than one hunk or file in the change. → extract the shared shape, call it from both.
- **Feature Envy**: a method that reaches into another object's data more than its own. → move the method onto the data it envies.
- **Data Clumps**: the same few fields or params keep travelling together (a type wanting to be born). → bundle them into one type, pass that.
- **Primitive Obsession**: a primitive or string standing in for a domain concept that deserves its own type. → give the concept its own small type.
- **Repeated Switches**: the same `switch`/`if`-cascade on the same type recurs across the change. → replace with polymorphism, or one map both sites share.
- **Shotgun Surgery**: one logical change forces scattered edits across many files in the diff. → gather what changes together into one module.
- **Divergent Change**: one file or module is edited for several unrelated reasons. → split so each module changes for one reason.
- **Speculative Generality**: abstraction, parameters, or hooks added for needs the spec doesn't have. → delete it; inline back until a real need shows.
- **Message Chains**: long `a.b().c().d()` navigation the caller shouldn't depend on. → hide the walk behind one method on the first object.
- **Middle Man**: a class or function that mostly just delegates onward. → cut it, call the real target direct.
- **Refused Bequest**: a subclass or implementer that ignores or overrides most of what it inherits. → drop the inheritance, use composition.

### 4. Run the two passes sequentially

In one session, run the Standards pass first, then the Spec pass — but keep their findings strictly separate. Do not let a Spec finding re-rank or soften a Standards finding or vice versa.

**Standards pass** checks, per file/hunk where relevant: (a) every place the diff violates a documented standard — cite the standard (file + the rule); and (b) any baseline smell you spot — name it and quote the hunk. Distinguish hard violations from judgement calls: documented-standard breaches can be hard, but baseline smells are always judgement calls, and a documented repo standard overrides the baseline. Skip anything tooling enforces.

**Spec pass** checks: (a) requirements the droplet and brief asked for that are missing or partial; (b) behaviour in the diff that wasn't asked for (scope creep); (c) requirements that look implemented but where the implementation looks wrong. Quote the spec line for each finding.

### 5. Aggregate

Present the two reports under `## Standards` and `## Spec` headings, verbatim or lightly cleaned. Do **not** merge or rerank findings, because the two axes are deliberately separate (see _Why two axes_).

End with a one-line summary: total findings per axis, and the worst issue _within each axis_ (if any). Don't pick a single winner across axes: that's the reranking the separation exists to prevent.

## Why two axes

A change can pass one axis and fail the other:

- Code that follows every standard but implements the wrong thing → **Standards pass, Spec fail.**
- Code that does exactly what the issue asked but breaks the project's conventions → **Spec pass, Standards fail.**

Reporting them separately stops one axis from masking the other.

---

## Attribution

Adapted for Cistern from Matt Pocock's [code-review](https://github.com/mattpocock/skills) skill
(v1.3.2, MIT, Copyright (c) 2026 Matt Pocock). Technique content adapted to cataractae
context; upstream keeps the canonical version. See `internal/skills/ATTRIBUTION.md`.

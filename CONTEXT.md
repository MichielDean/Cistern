# Context

## Item: ci-vlpsw

**Title:** Add docs cataracta between QA and delivery
**Status:** in_progress
**Priority:** 2

### Description

Add a documentation writer cataracta that runs between qa and delivery. Reviews the diff, finds documentation gaps, updates or creates docs in the repo.

Changes required:

1. cataractae_definitions: add docs_writer role. Instructions: read CONTEXT.md; run git diff main...HEAD; find all .md files; for each changed area (CLI, config, pipeline, architecture) check if docs exist and are accurate; if no user-visible changes pass with 'No documentation updates required'; otherwise update outdated sections, add missing docs, commit with '<id>: docs: update documentation for changes'; signal pass with file list or recirculate with specific ambiguity.

2. cataractae list: insert docs step between qa and delivery. Fields: name=docs, type=agent, identity=docs_writer, model=sonnet, context=full_codebase, skills=[cistern-droplet-state], timeout_minutes=20, skip_for=[1], on_pass=delivery, on_fail=implement, on_recirculate=implement, on_escalate=human

3. Routing: qa on_pass changes delivery -> docs. Trivial complexity skip_cataractae gains docs.

4. Mirror: copy updated aqueduct/feature.yaml to cmd/ct/assets/aqueduct/feature.yaml - both identical.

5. Tests: update any tests referencing cataracta list or pipeline step names.

## Current Step: implement

- **Type:** agent
- **Role:** implementer
- **Context:** full_codebase

## Recent Step Notes

### From: manual

All requirements verified complete: docs_writer definition in aqueduct.yaml, docs step with on_escalate=human, qa on_pass=docs, trivial skip_cataractae includes docs, both YAML mirrors identical, scheduler_test.go updated. No implementation changes needed. Committed CONTEXT.md (694d96f) to advance HEAD per scheduler requirement.

### From: manual

Implementation verified complete. All 5 requirements satisfied: (1) docs_writer cataracta definition in aqueduct.yaml; (2) docs step inserted between qa and delivery with on_escalate=human; (3) qa on_pass updated to docs; (4) trivial skip_cataractae includes docs; (5) both YAML mirrors identical. scheduler_test.go updated with docs step. Committed CONTEXT.md at 694d96f to advance HEAD.

### From: manual

Phase 2: aqueduct/aqueduct.yaml and cmd/ct/assets/aqueduct/aqueduct.yaml — docs step is missing on_escalate: human. The diff hunk @@ -420,6 +426,21 @@ inserts the docs step but its only routing fields are the three context lines inherited from the old security-review step (on_pass: delivery, on_fail: implement, on_recirculate: implement). The sole on_escalate: human addition in the diff goes to the security-review step, not the docs step. Critical droplets (complexity=4) at the docs step will not be escalated to human review before delivery. scheduler_test.go:231 has OnEscalate: 'human' because the workflow struct is constructed directly in Go — this masks the YAML deficiency. Fix: add on_escalate: human to the docs step in both YAML files.

### From: scheduler

Implement pass rejected: HEAD has not advanced since last review (commit: 694d96f4a4279810e8d1e22e9fbd1c92df9da704). No new commits were found. You must commit your changes before signaling pass.

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>.claude/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>github-workflow</name>
    <description>---</description>
    <location>.claude/skills/github-workflow/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-vlpsw

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-vlpsw
    ct droplet recirculate ci-vlpsw --to implement

**Block (genuinely blocked, cannot proceed):**
    ct droplet block ci-vlpsw

Add notes before signaling:
    ct droplet note ci-vlpsw "What you did / found"

The `ct` binary is on your PATH.

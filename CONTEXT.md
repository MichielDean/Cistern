# Context

## Item: ci-8ukna

**Title:** Add ct droplet stats command — show droplet counts grouped by status
**Status:** in_progress
**Priority:** 2

### Description

Add a new `ct droplet stats` subcommand under `ct droplet` that prints a summary of droplet counts grouped by status.

Expected output:
  flowing    2
  queued     1
  delivered  8
  stagnant   0
  ──────────────
  total      11

Requirements:
1. New cobra command `ct droplet stats` registered under dropletCmd
2. Queries the cistern DB via existing Client methods (use List or a new Stats method)
3. Counts by status: flowing (in_progress), queued (open), delivered, stagnant
4. Outputs a clean aligned table using tabwriter — status label on left, count right-aligned
5. Includes a separator line and total row
6. Exits 0 always (even if DB empty — just prints zeros)
7. Short description: 'Show droplet counts by status'
8. Full test coverage: TestDropletStats or similar using a tmp DB

Acceptance criteria (QA will verify):
- `ct droplet stats --help` shows the command and short description
- `ct droplet stats` runs without error on an empty DB
- `ct droplet stats` shows correct counts after seeding test data
- All existing tests still pass

## Current Step: implement

- **Type:** agent
- **Role:** implementer
- **Context:** full_codebase

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
    ct droplet pass ci-8ukna

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-8ukna
    ct droplet recirculate ci-8ukna --to implement

**Block (genuinely blocked, cannot proceed):**
    ct droplet block ci-8ukna

Add notes before signaling:
    ct droplet note ci-8ukna "What you did / found"

The `ct` binary is on your PATH.

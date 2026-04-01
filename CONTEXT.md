# Context

## Item: ci-gez0d

**Title:** ct filter: remove --file flag, plain-text output, leading questions
**Status:** in_progress
**Priority:** 2

### Description

Remove the --file flag from ct filter entirely. The finalize JSON step (filterFinalizePrompt) is lossy — the LLM drops depends_on when re-emitting the JSON array, causing all droplets to be filed without dependencies and dispatched simultaneously.

Changes:
- Remove --file and --repo flags from ct filter
- Delete filterFinalizePrompt constant and the --file branch in filter.go
- Remove addProposals and extractProposals if they have no other callers (check refine.go and cistern.go)
- Update filterSystemPrompt: output a numbered plain-text spec with prose dependency statements (e.g. '2. Implement Jira provider — requires droplet 1 to be delivered first') instead of a JSON array
- Update filterSystemPrompt: at each refinement round, ask leading questions to help the user sharpen the spec — e.g. probe for edge cases, unclear acceptance criteria, missing context, scope boundaries, and ordering rationale. The agent should drive the conversation toward a complete spec, not just wait for the user to volunteer information.
- Update runNonInteractive / response parsing accordingly — no more JSON parsing from stdout, just display the refined text and questions to the user
- Update filter_test.go and refine_test.go to reflect removed functionality

Acceptance criteria: ct filter starts a refinement conversation, asks probing questions at each round, and ends by printing a clear numbered plain-text spec with prose dependency statements. No --file flag exists. Filing is done separately by the caller using ct droplet add.

## Current Step: implement

- **Type:** agent
- **Role:** implementer
- **Context:** full_codebase

<available_skills>
  <skill>
    <name>cistern-droplet-state</name>
    <description>Manage droplet state in the Cistern agentic pipeline using the `ct` CLI.</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-droplet-state/SKILL.md</location>
  </skill>
  <skill>
    <name>cistern-git</name>
    <description>---</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-git/SKILL.md</location>
  </skill>
  <skill>
    <name>cistern-github</name>
    <description>---</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-github/SKILL.md</location>
  </skill>
</available_skills>

## Signaling Completion

When your work is done, signal your outcome using the `ct` CLI:

**Pass (work complete, move to next step):**
    ct droplet pass ci-gez0d

**Recirculate (needs rework — send back upstream):**
    ct droplet recirculate ci-gez0d
    ct droplet recirculate ci-gez0d --to implement

**Pool (cannot currently proceed):**
    ct droplet pool ci-gez0d

Add notes before signaling:
    ct droplet note ci-gez0d "What you did / found"

The `ct` binary is on your PATH.

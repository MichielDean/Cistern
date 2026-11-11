# Skill Attribution

The `mp-*` skills in this directory are adapted from Matt Pocock's public skills
collection: https://github.com/mattpocock/skills

- **Upstream version vendored:** v1.3.2 (commit `4ae2317`, 2026-11-11)
- **License:** MIT — Copyright (c) 2026 Matt Pocock
- **Canonical upstream SKILL.md files** live under `skills/{engineering,productivity,misc,in-progress}/`

## Adaptations

Upstream skills are conversation-oriented (a user drives an interactive session).
Cataractae are single-session pipeline workers with an identity layer (INSTRUCTIONS.md)
and no interactive user. Adaptations, per skill:

| Skill | Upstream source | Adaptation |
|---|---|---|
| mp-tdd | engineering/tdd | seams come from DESIGN_BRIEF.md instead of user agreement; supporting files (tests.md, mocking.md) inlined |
| mp-implement | engineering/implement | spec/ticket framing → droplet + brief-as-contract; review step removed (reviewer cataractae owns it) |
| mp-code-review | engineering/code-review | two parallel sub-agents → two sequential passes in one session; issue-tracker refs → droplet + CONTEXT.md |
| mp-codebase-design | engineering/codebase-design | supporting files (DEEPENING.md, DESIGN-IT-TWICE.md) inlined; parallel sub-agents → sequential designs |
| mp-domain-modeling | engineering/domain-modeling | supporting format files inlined; interactive user framing → brief/requirements framing |
| mp-diagnosing-bugs | engineering/diagnosing-bugs | user checkpoints → recirculation notes; hitl template reference replaced |
| mp-handoff | in-progress/handoff | $TMPDIR output → transient HANDOFF.md in worktree (recirculate payload) |
| mp-pr | engineering/pr | body kept nearly verbatim (upstream already credits Dex Horthy's show-me skill for the template) |
| mp-triage | engineering/triage | issue-tracker label state machine → droplet states + ct CLI commands; AGENT-BRIEF/OUT-OF-SCOPE condensed inline |
| mp-wizard | engineering/wizard | template.sh inlined verbatim (single-file deployment constraint) |

The MP-grilling methodology also drives the Cistern filtration prompt
(`cmd/ct/refine.go`), adapted there with attribution.

Platform-integration note: Cistern stages keep role identity in `cataractae/<role>/INSTRUCTIONS.md`;
these skills carry technique only, per the skills-vs-identity split in ARCHITECTURE.md.

## Refresh procedure

1. Clone https://github.com/mattpocock/skills at the new tag.
2. Re-apply the per-skill adaptations documented above to changed files.
3. Update the version/commit rows here and in each skill's attribution footer.

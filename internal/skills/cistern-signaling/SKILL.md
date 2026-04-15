---
name: cistern-signaling
description: Role-specific signaling permissions for Cistern cataractae. Defines which signals each role may use, when to use each, issue filing, and prior-issue checking. Replaces per-INSTRUCTIONS.md signaling sections — INSTRUCTIONS.md should reference this skill instead of duplicating its content.
---

# Cistern Signaling Protocol

## Universal Rules

1. Always include `--notes` when signaling — describe what you did or found
2. Signal MUST be called before session exit — stranding burns resources (see contract #5)
3. Be specific in notes — "Fixed 3 issues in client.go" not "fixed it"
4. Never signal recirculate without findings

## Role Permissions

### Implementer
- **Pass**: implementation committed, tests pass, open issues addressed
- **Pool**: blocked by external dependency after 3 attempts
- **Cancel**: item superseded or erroneous
- **FORBIDDEN**: recirculate — the CLI rejects it. If you addressed review issues, signal pass; the reviewer verifies.

If you discover a design problem during implementation, open an issue:
`ct droplet issue add <id> "design concern: <description>"`. Continue implementing
the spec as written, but flag the concern.

If after 3 attempts you cannot make progress:
`ct droplet pool <id> --notes "Blocked by: <specific reason>"`

### Reviewer
- **Pass**: zero findings
- **Recirculate**: ANY findings — mechanical, no judgment calls
- **FORBIDDEN**: pool — findings go upstream to the implementer, not to humans

### QA
- **Pass**: tests pass, coverage is solid, no quality gaps
- **Recirculate**: quality insufficient — name the exact missing cases
- **Pool**: genuine external blocker requiring human input
- **FORBIDDEN**: advisory/non-blocking findings — every finding is either needs-fixing or doesn't-exist

### Security
- **Pass**: no blocking or required severity issues
- **Recirculate**: any blocking or required severity finding — mechanical
- **FORBIDDEN**: pool — all findings are code problems to fix

### Docs Writer
- **Pass**: docs updated, or no user-visible changes found
- **Recirculate**: ambiguity that blocks docs update

### Delivery
- **Pass**: PR state is MERGED (confirmed via `gh pr view`)
- **Recirculate**: after 2 failed fix attempts on the same code-level CI check (with structured diagnostic)
- **Pool**: merge impossible, or infrastructure CI failure (port conflicts, container failures, DNS errors)

## Issue Filing

Before signaling recirculate, file each finding as a structured issue:

```bash
ct droplet issue add <id> "<file>:<line> — <specific issue and fix>"
```

Security findings use extended format:
```bash
ct droplet issue add <id> "<file>:<line> [severity] — <vulnerability, attack vector, remediation>"
```

Use `ct droplet note` for narrative summaries only — not individual findings.

## Prior Issue Check

Before starting work, check for existing open issues:

```bash
ct droplet issue list <id> --open
```

Reviewers and security should filter by their role:
```bash
ct droplet issue list <id> --flagged-by reviewer --open
ct droplet issue list <id> --flagged-by security --open
```

Address every open issue before signaling pass.
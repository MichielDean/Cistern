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

## Current Step: delivery

- **Type:** agent
- **Role:** delivery

## ⚠️ REVISION REQUIRED — Fix these issues before anything else

This droplet was recirculated. The following issues were found and **must** be fixed.
Do not proceed to implementation until you have read and understood each issue.

### Issue 1 (from: reviewer)

♻ 1 finding. filterSystemPrompt (refine.go:27-73) now instructs the LLM to produce plain-text numbered specs, but runNonInteractive (refine.go:129) — called from cistern.go:74 for ct droplet add --filter — still calls extractProposals which expects a JSON array. This will break ct droplet add --filter in production. Tests pass only because fakeagent returns canned JSON regardless of prompt. See issue ci-gez0d-954y2 for details and fix options.

### Issue 2 (from: reviewer)

No findings. Phase 1: resolved ci-gez0d-954y2 — broken code path (runNonInteractive/extractProposals) completely removed. Phase 2: fresh review found no security vulnerabilities, logic errors, missing error handling, missing tests, API contract violations, or resource leaks. All removed symbols verified zero-referenced. Tests pass.

### Issue 3 (from: qa)

♻ Tests pass. Phase 1: both prior issues resolved — runNonInteractive, extractProposals, addProposals, DropletProposal, and TUI are gone with zero references. Phase 2: one finding (ci-gez0d-oguh6). printFilterResult is the scripting interface (--output-format json) and its tests only assert err==nil — no test captures stdout and decodes the JSON to verify session_id and text field names. TestFilterJSONOutput_HasRequiredFields tests a local shadow struct, not the production anonymous jsonOut, so a tag rename in the production code would pass all three tests silently. Fix: redirect stdout in the two printFilterResult tests, decode the captured JSON, and assert session_id and text are present. Similarly assert the human-format path emits result.Text to stdout and result.SessionID to stderr.

### Issue 4 (from: reviewer)

Phase 1: resolved ci-gez0d-oguh6 — printFilterResult tests now capture stdout/stderr via os.Pipe and verify session_id and text fields against production output. Shadow struct test deleted. Phase 2: fresh adversarial review found no new issues. All removed symbols (filterFile, filterRepo, addFilter, addYes, extractProposals, addProposals, DropletProposal, runNonInteractive, filterFinalizePrompt, TUI code) confirmed zero-referenced. DropletAdder interface change is internal-only with updated test doubles. Jira provider uses url.PathEscape on user input, LimitReader, and proper auth. SetExternalRef validates format with regex and git-safety checks. branchForDroplet handles ExternalRef correctly. Build ok. All 15 packages pass.

### Issue 5 (from: qa)

♻ Tests pass. Phase 1: ci-gez0d-oguh6 resolved — printFilterResult tests capture stdout/stderr via os.Pipe and assert session_id and text field values against production output. Shadow struct test deleted. All prior issues confirmed resolved.

Phase 2: four stale comments reference functions removed by this change. Per QA protocol, incorrect comments are a recirculate.

ci-gez0d-dcd59: fakeagent/main.go:20-22 — 'This preserves backward compatibility with runNonInteractive() in refine.go.' runNonInteractive was removed. The raw-output path is now exercised only by the FAKEAGENT_MODE=raw_fallback test case for callFilterAgent's JSON-fallback path. Update the comment accordingly.

ci-gez0d-zdsiy: failagent/main.go:3 — package comment says 'testing error handling in runNonInteractive'; runNonInteractive was removed. failagent now tests the exec-failure path in callFilterAgent. Update the comment.

ci-gez0d-1arwq: mockllm/mockllm.go:21-22 — package-level usage example calls callRefineAPI("Fix login bug", "") which does not exist in the codebase. Remove or replace with an example reflecting current test usage.

ci-gez0d-54tbr: refine_test.go:44 — TestMockLLM_RecordsRequestsForAllProviders comment says 'how a caller would configure callRefineAPI for that provider'; callRefineAPI does not exist. Remove the reference.

### Issue 6 (from: reviewer)

Phase 1: resolved all 4 QA issues — (1) ci-gez0d-dcd59: fakeagent/main.go:19-22 now correctly describes FAKEAGENT_MODE=raw_fallback / callFilterAgent JSON-fallback, no runNonInteractive reference. (2) ci-gez0d-zdsiy: failagent/main.go:3 now says 'exec-failure path in callFilterAgent'. (3) ci-gez0d-1arwq: mockllm/mockllm.go doc example removed callRefineAPI, replaced with generic mock server usage. (4) ci-gez0d-54tbr: refine_test.go:42-44 removed callRefineAPI reference.

Phase 2 fresh review: no new findings. Removed symbols (filterFile, filterRepo, addFilter, addYes, extractProposals, addProposals, DropletProposal, runNonInteractive, filterFinalizePrompt, complexityToInt, TUI code, charmbracelet imports from refine.go) all confirmed zero-referenced. New code (tracker interface, jira provider, ExternalRef field, branchForDroplet, SetExternalRef validation, DropletAdder interface update) reviewed for security, logic, error handling, resource management, and contract consistency. Jira provider uses url.PathEscape on user input, io.LimitReader(1MiB), Basic Auth, and 30s timeout. SetExternalRef validates format with regex and git-safety checks. branchForDroplet handles all ExternalRef cases (set/empty/malformed). Build ok. All 15 packages pass.

### Issue 7 (from: reviewer)

Phase 1: all 4 QA issues resolved (stale comments fixed in fakeagent, failagent, mockllm, refine_test). Phase 2: fresh adversarial review — no new findings. All removed symbols zero-referenced. New tracker/jira/ExternalRef/branchForDroplet code reviewed for security, logic, error handling, contracts. Build ok, all 15 packages pass.

### Issue 8 (from: qa)

Phase 1: no open QA issues. Phase 2: fresh review — no new findings. All removed symbols zero-referenced. Simplifier's latest commit (50016bd) accurately updated hardcodedProposals and hardcodedJSONEnvelope comments in fakeagent/main.go. Test coverage solid: error paths, JSON fallback, is_error envelope, missing env var, all three removed-flag rejection tests, stdout/stderr capture in printFilterResult tests, AllowedTools passthrough. filterSystemPrompt is plain-text with probing questions. All acceptance criteria have tests. Build ok, all 15 packages pass.

### Issue 9 (from: security)

Phase 1: no open security issues. All prior issues confirmed resolved through 8 review cycles. Phase 2: fresh adversarial security review of full diff — no findings. SQL parameterized throughout. callFilterAgent uses slice-based exec args. Jira provider uses url.PathEscape, io.LimitReader(1MiB), 30s timeout, Basic Auth with env-var token preference. SetExternalRef validates with regex + git-safety checks; branchForDroplet safe. HTTP handler retains MaxBytesReader, rate limiting, bearer auth. No secrets in error messages. AllowedTools hardcoded to read-only. Attack surface reduced: removed extractProposals JSON parsing, TUI code, --file/--repo flags. Build ok, all 15 packages pass.

### Issue 10 (from: security)

No security issues found. Diff reduces attack surface (removed extractProposals, TUI, --file/--repo). New code (Jira provider, ExternalRef, branchForDroplet) follows security best practices: parameterized SQL, slice-based exec, url.PathEscape, LimitReader, regex validation, no secret leakage. Build ok, all 15 packages pass.

---

## Recent Step Notes

### From: docs_writer

Documentation complete. All user-visible changes documented: Jira Cloud tracker integration (CHANGELOG, README, configs), removal of --file/--repo flags from ct filter (SKILL.md, commands.md), external_ref support for PR titles (CLAUDE.md). Fixed stale flag references in commands.md reference docs.

### From: security

No security issues found. Diff reduces attack surface (removed extractProposals, TUI, --file/--repo). New code (Jira provider, ExternalRef, branchForDroplet) follows security best practices: parameterized SQL, slice-based exec, url.PathEscape, LimitReader, regex validation, no secret leakage. Build ok, all 15 packages pass.

### From: security

Phase 1: no open security issues. All prior issues confirmed resolved through 8 review cycles. Phase 2: fresh adversarial security review of full diff — no findings. SQL parameterized throughout. callFilterAgent uses slice-based exec args. Jira provider uses url.PathEscape, io.LimitReader(1MiB), 30s timeout, Basic Auth with env-var token preference. SetExternalRef validates with regex + git-safety checks; branchForDroplet safe. HTTP handler retains MaxBytesReader, rate limiting, bearer auth. No secrets in error messages. AllowedTools hardcoded to read-only. Attack surface reduced: removed extractProposals JSON parsing, TUI code, --file/--repo flags. Build ok, all 15 packages pass.

### From: qa

Phase 1: no open QA issues. Phase 2: fresh review — no new findings. All removed symbols zero-referenced. Simplifier's latest commit (50016bd) accurately updated hardcodedProposals and hardcodedJSONEnvelope comments in fakeagent/main.go. Test coverage solid: error paths, JSON fallback, is_error envelope, missing env var, all three removed-flag rejection tests, stdout/stderr capture in printFilterResult tests, AllowedTools passthrough. filterSystemPrompt is plain-text with probing questions. All acceptance criteria have tests. Build ok, all 15 packages pass.

<available_skills>
  <skill>
    <name>cistern-github</name>
    <description>---</description>
    <location>/home/lobsterdog/.cistern/skills/cistern-github/SKILL.md</location>
  </skill>
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
